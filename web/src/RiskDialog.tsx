import { msg } from "./locale";
import type { ReactNode } from "react";
import { useRef, useState } from "react";
import { Dialog } from "@rosemary-ui/react";
import { AlertTriangle, ShieldCheck } from "./icons";
import { Button, clearCompletedOverlayMotion } from "./Forma";
import { useVersionHold } from "./VersionGuard";
export function RiskDialog({
  eyebrow = msg("风险确认"),
  title,
  lead,
  children,
  busy = false,
  confirmDisabled = false,
  cancelLabel = msg("取消"),
  confirmLabel = msg("确认"),
  onCancel,
  onConfirm,
}: {
  eyebrow?: string;
  title: string;
  lead?: string;
  children: ReactNode;
  busy?: boolean;
  confirmDisabled?: boolean;
  cancelLabel?: string;
  confirmLabel?: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  useVersionHold();
  const [open, setOpen] = useState(true);
  const cancelRef = useRef<HTMLButtonElement>(null);
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value && !busy) setOpen(false);
      }}
      title={
        <>
          <span className="risk-icon">
            <AlertTriangle />
          </span>
          {title}
        </>
      }
      description={lead}
      className={`risk-dialog ${busy ? "is-busy" : ""}`}
      closeLabel={msg("取消并返回")}
      onAnimationComplete={(value) => {
        if (value) clearCompletedOverlayMotion();
        else onCancel();
      }}
      footer={
        <>
          <Button
            ref={cancelRef}
            autoFocus
            variant="secondary"
            disabled={busy}
            onClick={() => setOpen(false)}
          >
            {cancelLabel}
          </Button>
          <Button
            variant="danger"
            loading={busy}
            disabled={confirmDisabled}
            onClick={onConfirm}
          >
            <ShieldCheck />
            {confirmLabel}
          </Button>
        </>
      }
    >
      <p className="eyebrow">{eyebrow}</p>
      {children}
    </Dialog>
  );
}
