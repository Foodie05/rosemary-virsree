import { msg } from "./locale";
import { LanguageToggle, useLanguage } from "./locale";
import {
  createContext,
  forwardRef,
  useContext,
  useState,
  useSyncExternalStore,
} from "react";
import type { ReactNode } from "react";
import {
  Button as FormaButton,
  Dialog,
  IconButton,
  RosemaryProvider,
  useRosemaryTheme,
} from "@rosemary-ui/react";
import type { ButtonProps } from "@rosemary-ui/react";
export {
  Input,
  Select,
  Checkbox,
  Textarea,
  Loading,
  Skeleton,
  Progress,
  Tabs,
  Breadcrumb,
  Stepper,
  Alert,
  Empty,
  IconButton,
} from "@rosemary-ui/react";
const AppearanceContext = createContext<() => void>(() => {});
export function AppearanceProvider({ children }: { children: ReactNode }) {
  const language = useLanguage();
  const [mode, setMode] = useState<"system" | "light" | "dark">(() => {
    try {
      const saved =
        localStorage.getItem("arca-theme") ||
        localStorage.getItem("virsree-appearance");
      return saved === "light" || saved === "dark" ? saved : "system";
    } catch {
      return "system";
    }
  });
  const toggle = () =>
    setMode((current) => {
      const effective =
        current === "system"
          ? matchMedia("(prefers-color-scheme: dark)").matches
            ? "dark"
            : "light"
          : current;
      const next = effective === "dark" ? "light" : "dark";
      try {
        localStorage.setItem("arca-theme", next);
      } catch {}
      return next;
    });
  return (
    <RosemaryProvider mode={mode} language={language}>
      <AppearanceContext.Provider value={toggle}>
        {children}
      </AppearanceContext.Provider>
    </RosemaryProvider>
  );
}
export function ThemeToggle() {
  const { mode } = useRosemaryTheme();
  const toggle = useContext(AppearanceContext);
  const systemDark = useSyncExternalStore(
    (listener) => {
      const media = matchMedia("(prefers-color-scheme: dark)");
      media.addEventListener("change", listener);
      return () => media.removeEventListener("change", listener);
    },
    () => matchMedia("(prefers-color-scheme: dark)").matches,
    () => false,
  );
  const dark = mode === "dark" || (mode === "system" && systemDark);
  return (
    <div className="appearance-controls">
      <LanguageToggle />
      <IconButton
        className="theme-toggle"
        icon={dark ? "sun" : "moon"}
        label={dark ? msg("切换为浅色外观") : msg("切换为深色外观")}
        variant="ghost"
        onClick={toggle}
      />
    </div>
  );
}

// Keep the existing form submission semantics while all buttons share Forma's
// physical spring, busy handling, optical inset and keyboard focus contract.
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button({ className = "", variant, type = "submit", ...props }, ref) {
    const tone =
      variant ||
      (/danger-confirm|bucket-delete-button|\bdanger\b|revoke/.test(className)
        ? "danger"
        : /\bprimary\b/.test(className)
          ? "primary"
          : /\bsecondary\b/.test(className)
            ? "secondary"
            : "ghost");
    return (
      <FormaButton
        {...props}
        ref={ref}
        type={type}
        className={className}
        variant={tone}
      />
    );
  },
);
export function clearCompletedOverlayMotion() {
  for (const dialog of document.querySelectorAll("dialog[open]"))
    for (const animation of dialog.getAnimations())
      if (animation.playState === "finished") animation.cancel();
}
export function Modal({
  title,
  close,
  children,
  dismissible = true,
  busy = false,
  className = "",
  returnFocusRef,
}: {
  title: string;
  close: () => void;
  children: ReactNode;
  dismissible?: boolean;
  busy?: boolean;
  className?: string;
  returnFocusRef?: {
    readonly current: HTMLElement | null;
  };
}) {
  const [open, setOpen] = useState(true);
  const requestClose = () => {
    if (dismissible && !busy) setOpen(false);
  };
  return (
    <Dialog
      returnFocusRef={returnFocusRef}
      open={open}
      onOpenChange={(value) => {
        if (!value) requestClose();
      }}
      title={title}
      className={`virsree-modal ${!dismissible ? "required-dialog" : ""} ${busy ? "is-busy" : ""} ${className}`}
      onAnimationComplete={(value) => {
        if (value) clearCompletedOverlayMotion();
        else close();
      }}
    >
      {children}
    </Dialog>
  );
}
