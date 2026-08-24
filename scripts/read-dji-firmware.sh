#!/usr/bin/env bash
set -Eeuo pipefail
export LC_ALL=C

mode=check
device=/dev/ttyUSB2

usage() {
  cat <<'EOF'
Usage: read-dji-firmware.sh [--check | --read] [--device /dev/ttyUSB<N>]

Read only ATI, AT+CGMM, and AT+CGMR from the DJI modem's AT interface. Raw
responses are never printed or logged. Output is restricted to sanitized
manufacturer, model, and firmware fields; identifier-like or sensitive lines
are discarded.

--check is the default and sends no AT command. ModemManager must be disabled.
EOF
}

die() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

while (($#)); do
  case "$1" in
    --check|--read)
      mode=${1#--}
      shift
      ;;
    --device)
      (($# >= 2)) || die '--device requires a value'
      device=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

[[ $device =~ ^/dev/ttyUSB[0-9]{1,2}$ ]] || die 'device must be a ttyUSB port'
[[ -c $device ]] || die 'AT device is not a character device'
[[ -r $device && -w $device ]] || die 'AT device is not readable and writable by the current user'
for command_name in dirname readlink systemctl; do
  command -v "$command_name" >/dev/null 2>&1 || die "required command is missing: $command_name"
done

tty_name=${device#/dev/}
tty_sysfs=$(readlink -f -- "/sys/class/tty/$tty_name/device") || die 'cannot resolve the AT port sysfs identity'
[[ $tty_sysfs == /sys/devices/* ]] || die 'AT port does not resolve to a kernel sysfs device'
probe=$tty_sysfs
usb_interface=
usb_device=
while [[ $probe == /sys/devices/* && $probe != /sys/devices ]]; do
  parent=$(dirname -- "$probe")
  if [[ -r $probe/bInterfaceNumber && -r $parent/idVendor && -r $parent/idProduct ]]; then
    usb_interface=$probe
    usb_device=$parent
    break
  fi
  probe=$parent
done
[[ -n $usb_interface && -n $usb_device ]] || die 'AT port is not attached to a USB interface'
vendor_id=$(<"$usb_device/idVendor")
product_id=$(<"$usb_device/idProduct")
interface_number=$(<"$usb_interface/bInterfaceNumber")
[[ ${vendor_id,,} == 2ca3 && ${product_id,,} == 4006 ]] || die 'AT port does not belong to the reviewed DJI 2ca3:4006 device'
[[ ${interface_number,,} == 02 ]] || die 'AT port is not DJI interface 2'
if systemctl is-active --quiet ModemManager.service 2>/dev/null; then
  die 'ModemManager is active and may race for the AT port'
fi
if systemctl is-active --quiet vocat.service 2>/dev/null; then
  die 'vocat.service is active; stop it during the firmware-read maintenance window'
fi

if [[ $mode == check ]]; then
  printf 'AT-port check passed; no modem command was sent.\n'
  exit 0
fi

command -v python3 >/dev/null 2>&1 || die 'python3 is required'
python3 - "$device" <<'PY'
import fcntl
import os
import re
import select
import sys
import termios
import time

device = sys.argv[1]
blocked_words = re.compile(
    r"(?:IMEI|ICCID|IMSI|MSISDN|PHONE|SIM\s*PIN|SMS|MESSAGE|SUBSCRIBER)",
    re.IGNORECASE,
)
known_vendors = {
    "baiwang",
    "dji",
    "fibocom",
    "meig",
    "quectel",
    "qualcomm",
    "simcom",
}


def fail(message):
    raise SystemExit(f"ERROR: {message}")


def configure_port(fd):
    attrs = termios.tcgetattr(fd)
    attrs[0] = 0
    attrs[1] = 0
    attrs[2] = termios.CLOCAL | termios.CREAD | termios.CS8
    attrs[3] = 0
    attrs[4] = termios.B115200
    attrs[5] = termios.B115200
    attrs[6][termios.VMIN] = 0
    attrs[6][termios.VTIME] = 0
    termios.tcsetattr(fd, termios.TCSANOW, attrs)
    termios.tcflush(fd, termios.TCIOFLUSH)


def query(fd, command):
    termios.tcflush(fd, termios.TCIFLUSH)
    os.write(fd, (command + "\r").encode("ascii"))
    deadline = time.monotonic() + 4.0
    response = bytearray()
    while time.monotonic() < deadline and len(response) < 16384:
        readable, _, _ = select.select([fd], [], [], min(0.25, deadline - time.monotonic()))
        if not readable:
            continue
        chunk = os.read(fd, 4096)
        if chunk:
            response.extend(chunk)
        normalized = bytes(response).replace(b"\r", b"\n")
        if b"\nOK\n" in normalized or b"\nERROR\n" in normalized or b"+CME ERROR" in normalized:
            break
    text = response.decode("ascii", errors="replace")
    lines = [line.strip() for line in re.split(r"[\r\n]+", text) if line.strip()]
    if not any(line == "OK" for line in lines):
        fail(f"{command} did not return a successful status")
    return [line for line in lines if line not in {command, "OK"}]


def sanitize(value, field, vendor=False):
    value = value.strip().strip('"')
    value = re.sub(
        r"^(?:(?:Manufacturer|Model|Revision|Firmware)|\+CGMM|\+CGMR)\s*:\s*",
        "",
        value,
        flags=re.IGNORECASE,
    )
    if not value or len(value) > 128 or blocked_words.search(value):
        return None
    if any(ord(char) < 32 or ord(char) > 126 for char in value):
        return None
    # Long decimal runs are modem/SIM/account identifiers far more often than
    # useful model or firmware labels. Discard the whole field instead of
    # attempting a partial redaction that could still disclose it.
    if re.search(r"\d{8,}", value):
        return None
    if vendor:
        if (
            len(value) > 64
            or not re.fullmatch(r"[A-Za-z][A-Za-z .,_()/+-]*", value)
            or value.casefold() not in known_vendors
        ):
            return None
    else:
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9 .,_()/+:-]*", value):
            return None
        if not re.search(r"[A-Za-z]", value) or not re.search(r"[0-9]", value):
            return None
    return value


def first_safe(lines, field, vendor=False, skip=()):
    for line in lines:
        candidate = sanitize(line, field, vendor=vendor)
        if candidate and candidate not in skip:
            return candidate
    return "unavailable"


def single_safe(lines, field, expected_prefix, skip=()):
    # CGMM/CGMR are single-value commands. Any extra line is treated as an
    # unsolicited result code and suppresses the whole field rather than
    # risking disclosure of a message or identifier from another modem event.
    if len(lines) != 1 or contains_unsolicited(lines, expected_prefix):
        return "unavailable"
    return first_safe(lines, field, skip=skip)


def contains_unsolicited(lines, expected_prefix=None):
    for line in lines:
        upper = line.upper()
        if blocked_words.search(line) or upper in {"RING", "NO CARRIER"}:
            return True
        if line.startswith("+") and not (expected_prefix and upper.startswith(expected_prefix)):
            return True
    return False


fd = os.open(device, os.O_RDWR | os.O_NOCTTY | os.O_NONBLOCK)
original_attrs = termios.tcgetattr(fd)
try:
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        fail("AT device is already in use")
    configure_port(fd)
    ati_lines = query(fd, "ATI")
    model_lines = query(fd, "AT+CGMM")
    revision_lines = query(fd, "AT+CGMR")

    model = single_safe(model_lines, "model", "+CGMM")
    firmware = single_safe(revision_lines, "firmware", "+CGMR", skip=(model,))
    manufacturer = "unavailable"
    if not contains_unsolicited(ati_lines):
        labelled_vendor = [line for line in ati_lines if re.match(r"^Manufacturer\s*:", line, re.IGNORECASE)]
        manufacturer = first_safe(labelled_vendor, "manufacturer", vendor=True)
        if manufacturer == "unavailable":
            manufacturer = first_safe(ati_lines, "manufacturer", vendor=True, skip=(model, firmware))

    print(f"manufacturer: {manufacturer}")
    print(f"model: {model}")
    print(f"firmware: {firmware}")
finally:
    termios.tcsetattr(fd, termios.TCSANOW, original_attrs)
    os.close(fd)
PY
