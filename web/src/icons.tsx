import { Icon, Loading } from "@rosemary-ui/react";
import type { IconProps } from "@rosemary-ui/react";
import type { IconName } from "@rosemary-ui/icons";
const icon = (name: IconName) =>
  function ProductIcon(props: Omit<IconProps, "name">) {
    return <Icon {...props} name={name} size={props.size ?? 20} />;
  };
export const Activity = icon("branch"),
  BookOpen = icon("book"),
  Box = icon("archive"),
  ChevronRight = icon("chevron"),
  Cloud = icon("cloud"),
  Copy = icon("copy"),
  Database = icon("archive"),
  KeyRound = icon("key"),
  LayoutDashboard = icon("grid"),
  LogOut = icon("arrow-left"),
  Pencil = icon("pencil"),
  Plus = icon("plus"),
  RefreshCw = icon("refresh"),
  ShieldCheck = icon("shield-check"),
  Sparkles = icon("compass"),
  Trash2 = icon("trash"),
  X = icon("close"),
  ArrowRight = icon("arrow"),
  Check = icon("check"),
  Globe2 = icon("globe"),
  HardDrive = icon("folder"),
  Waypoints = icon("branch"),
  AlertTriangle = icon("warning"),
  ArrowLeft = icon("arrow-left"),
  Download = icon("download"),
  File = icon("file"),
  Folder = icon("folder"),
  Search = icon("search"),
  Clock3 = icon("clock"),
  Clipboard = icon("copy"),
  Code2 = icon("text"),
  ExternalLink = icon("external-link"),
  FileKey = icon("key"),
  Link2 = icon("link"),
  LockKeyhole = icon("lock"),
  Network = icon("branch"),
  Terminal = icon("text"),
  Upload = icon("upload");
// Legacy import name intentionally maps to the official folding loader; no
// rotating SVG or brand mark remains in request states.
export function Loader2({
  className = "",
  size,
}: {
  className?: string;
  size?: number;
}) {
  return (
    <Loading
      label="正在处理"
      size={size && size < 20 ? "small" : "medium"}
      className={className.replace(/\bspin\b/g, "")}
      delay={0}
    />
  );
}
