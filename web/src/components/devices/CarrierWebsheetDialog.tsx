import { useEffect, useRef, useState } from "react";
import { Modal } from "../ui";
import { useI18n } from "../../lib/i18n";

export interface CarrierWebsheet {
  id?: string;
  title?: string;
  embedUrl?: string;
}

export interface CarrierWebsheetDialogProps {
  open: boolean;
  websheet: CarrierWebsheet | null;
  onClose: () => void;
  onDone: () => void;
}

export function CarrierWebsheetDialog({ open, websheet, onClose, onDone }: CarrierWebsheetDialogProps) {
  const { t } = useI18n();
  const [loaded, setLoaded] = useState(false);
  const doneRef = useRef(false);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const embedUrl = websheet?.embedUrl || "";

  useEffect(() => {
    setLoaded(false);
    doneRef.current = false;
  }, [websheet?.id]);

  useEffect(() => {
    function isValid(data: unknown): data is { type: string } {
      if (!data || typeof data !== "object") return false;
      const d = data as Record<string, unknown>;
      return d.type === "vohive-websheet-callback";
    }
    function done() {
      if (doneRef.current) return;
      doneRef.current = true;
      onDone();
      onClose();
    }
    const onMessage = (event: MessageEvent) => {
      if (!open || event.origin !== window.location.origin) return;
      if (event.source !== iframeRef.current?.contentWindow) return;
      if (isValid(event.data)) done();
    };
    window.addEventListener("message", onMessage);
    return () => {
      window.removeEventListener("message", onMessage);
    };
  }, [open, websheet?.id, onClose, onDone]);

  return (
    <Modal open={open} onClose={onClose} title={websheet?.title || t("E911地址")} width="max-w-[min(390px,94vw)]">
      <div className="websheet-frame-shell relative overflow-hidden rounded border border-gray-200 dark:border-gray-700">
        {!loaded ? (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-white/80 text-sm text-gray-500 dark:bg-gray-900/80">
            {t("加载中...")}
          </div>
        ) : null}
        {embedUrl ? (
          <iframe
            ref={iframeRef}
            src={embedUrl}
            title={websheet?.title || t("E911地址")}
            className="block h-full w-full border-0"
            sandbox="allow-forms allow-same-origin allow-scripts"
            onLoad={() => setLoaded(true)}
          />
        ) : null}
      </div>
    </Modal>
  );
}
