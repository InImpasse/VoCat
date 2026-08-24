import type { ReactNode } from "react";
import {
  CheckmarkRegular,
  InfoRegular,
  KeyRegular,
} from "@fluentui/react-icons";
import type { SystemInfo } from "../../types";
import { useI18n } from "../../lib/i18n";
import { Button } from "../ui/Button";
import { FieldRow, PasswordInput } from "./controls";

export interface PasswordForm {
  oldPassword: string;
  newPassword: string;
  confirmPassword: string;
}

function CardDecor() {
  return (
    <div className="absolute right-0 top-0 -mr-10 -mt-10 h-40 w-40 rounded-bl-full bg-indigo-500/5 transition-transform group-hover:scale-110" />
  );
}

function CardIcon({ children, small }: { children: ReactNode; small?: boolean }) {
  return (
    <div
      className={
        small
          ? "flex h-9 w-9 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400"
          : "flex h-12 w-12 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400"
      }
    >
      {children}
    </div>
  );
}

function CardTitle({ title, subtitle }: { title: string; subtitle: string }) {
  return (
    <div>
      <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100">{title}</h3>
      <p className="text-xs text-gray-500">{subtitle}</p>
    </div>
  );
}

const PASSWORD_LABEL = "text-xs font-bold uppercase tracking-wider text-gray-500";

export function SecurityCard({
  value,
  onChange,
  loading,
  onSubmit,
}: {
  value: PasswordForm;
  onChange: (patch: Partial<PasswordForm>) => void;
  loading: boolean;
  onSubmit: () => void;
}) {
  const { t } = useI18n();
  return (
    <div className="ui-card group relative overflow-hidden p-8">
      <CardDecor />
      <div className="relative z-10 mb-6 flex items-center gap-3">
        <CardIcon>
          <KeyRegular className="text-[24px]" />
        </CardIcon>
        <CardTitle title={t("安全")} subtitle={t("更新访问凭证")} />
      </div>
      <div className="relative z-10 space-y-4">
        <div className="space-y-1">
          <label className={PASSWORD_LABEL}>{t("当前密码")}</label>
          <PasswordInput
            inputSize="large"
            placeholder="••••••••"
            autoComplete="current-password"
            value={value.oldPassword}
            onChange={(oldPassword) => onChange({ oldPassword })}
          />
        </div>
        <div className="space-y-1">
          <label className={PASSWORD_LABEL}>{t("新密码")}</label>
          <PasswordInput
            inputSize="large"
            placeholder="••••••••"
            autoComplete="new-password"
            value={value.newPassword}
            onChange={(newPassword) => onChange({ newPassword })}
          />
        </div>
        <div className="space-y-1">
          <label className={PASSWORD_LABEL}>{t("确认新密码")}</label>
          <PasswordInput
            inputSize="large"
            placeholder="••••••••"
            autoComplete="new-password"
            value={value.confirmPassword}
            onChange={(confirmPassword) => onChange({ confirmPassword })}
          />
        </div>
        <div className="pt-4">
          <Button variant="primary" size="large" loading={loading} onClick={onSubmit} className="w-full !border-0" icon={<CheckmarkRegular />}>
            {t("更新凭证")}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function SystemInfoCard({
  info,
}: {
  info: SystemInfo;
}) {
  const { t } = useI18n();
  return (
    <div className="ui-card group relative overflow-hidden p-8">
      <CardDecor />
      <div className="relative z-10 mb-6 flex items-center gap-3">
        <CardIcon>
          <InfoRegular className="text-[24px]" />
        </CardIcon>
        <CardTitle title={t("系统信息")} subtitle={t("运行环境")} />
      </div>
      <div className="relative z-10 space-y-4 text-sm">
        <div className="rounded-lg bg-gray-50 p-3 dark:bg-white/5">
          <FieldRow label={t("版本")} value={info.version || "Unknown"} monospace />
        </div>
        <div className="rounded-lg bg-gray-50 p-3 dark:bg-white/5">
          <FieldRow label={t("构建时间")} value={info.buildTime} monospace />
        </div>
        <div className="rounded-lg bg-gray-50 p-3 dark:bg-white/5">
          <FieldRow label={t("配置路径")} value={info.config} monospace copyable />
        </div>
        <div className="rounded-lg bg-gray-50 p-3 dark:bg-white/5">
          <FieldRow label={t("运行时长")} value={info.uptime} monospace />
        </div>
        <div className="rounded-lg bg-gray-50 p-3 dark:bg-white/5">
          <FieldRow label="OS" value={info.os} monospace />
        </div>
        <div className="rounded-lg bg-gray-50 p-3 dark:bg-white/5">
          <FieldRow label={t("架构")} value={info.architecture} monospace />
        </div>
      </div>
    </div>
  );
}

export { CardDecor, CardIcon, CardTitle };
