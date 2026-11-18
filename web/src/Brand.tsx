import { RosemaryLogo } from "@rosemary-ui/react";
export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className="brand">
      <RosemaryLogo alt="" width={44} height={44} />
      {!compact && (
        <div>
          <strong>Arca</strong>
          <span>Rosemary</span>
        </div>
      )}
    </div>
  );
}
