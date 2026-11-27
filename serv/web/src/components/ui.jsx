import React from "react";
import { useInRouterContext, useLocation } from "react-router-dom";
import { CircleAlert, CircleCheck, CircleDashed, CircleX, Loader2, Sparkle, TriangleAlert } from "lucide-react";

import { routeDefinition } from "@/app/workspaces";
import { cn } from "@/lib/utils";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";

export const cx = cn;

// PageHeader takes its icon and colour from the view at the current address,
// so it matches the rail without each page repeating them. Pages rendered
// alone (in tests) have no router and get a plain header.
export function PageHeader({ eyebrow, title, description, actions, icon, tone }) {
  const inRouter = useInRouterContext();
  return inRouter
    ? <RoutedPageHeader eyebrow={eyebrow} title={title} description={description} actions={actions} icon={icon} tone={tone} />
    : <PageHeaderView eyebrow={eyebrow} title={title} description={description} actions={actions} icon={icon} tone={tone} />;
}

function RoutedPageHeader(props) {
  const route = routeDefinition(useLocation().pathname);
  return <PageHeaderView {...props} icon={props.icon || route?.icon} tone={props.tone || route?.tone} />;
}

function PageHeaderView({ eyebrow, title, description, actions, icon: Icon, tone }) {
  return (
    <div data-tone={tone} className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
      <div className="flex min-w-0 items-center gap-4">
        {Icon && (
          <span aria-hidden="true" className="tone-tile">
            <Icon className="size-6 md:size-7" strokeWidth={1.75} />
          </span>
        )}
        <div className="grid min-w-0 gap-1">
          {eyebrow && <p className="hud-label">{eyebrow}</p>}
          <h1 className="font-display text-[2rem] leading-[1.08] md:text-[2.5rem]">{title}</h1>
          {description && <p className="max-w-3xl text-[0.9375rem] leading-6 text-muted-foreground">{description}</p>}
        </div>
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}

export function Panel({ title, description, children, action, className, contentClassName, icon: Icon, tone }) {
  return (
    <section data-tone={tone} className={cx("min-w-0", className)}>
      {(title || action) && (
        <header className="flex flex-col gap-3 pt-2 pb-1 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-2.5">
            {Icon && <span aria-hidden="true" className="tone-tile"><Icon className="size-4" /></span>}
            <div className="min-w-0">
              {title && <h2 className="font-display text-[1.25rem] leading-tight tracking-[-0.03em]">{title}</h2>}
              {description && <p className="mt-1 font-mono text-xs leading-5 text-muted-foreground">{description}</p>}
            </div>
          </div>
          {action}
        </header>
      )}
      <div className={cx("py-3", !(title || action) && "py-4", contentClassName)}>{children}</div>
    </section>
  );
}

const metricTones = {
  good: { text: "text-success", tone: "teal", icon: CircleCheck },
  warn: { text: "text-warning", tone: "amber", icon: TriangleAlert },
  bad: { text: "text-destructive", tone: "warm", icon: CircleX },
  neutral: { text: "text-muted-foreground", tone: "blue", icon: null },
};

export function Metric({ label, value, detail, tone = "neutral", icon, className }) {
  const look = metricTones[tone] || metricTones.neutral;
  const Icon = icon || look.icon;
  return (
    <div data-tone={look.tone} className={cx("flex min-w-0 flex-col gap-2 py-3", className)}>
      <div className="flex items-center gap-1.5">
        {Icon && <Icon aria-hidden="true" className="size-3.5 shrink-0 text-[var(--tone)]" />}
        <span className={cx("hud-label", look.text)}>{label}</span>
      </div>
      <strong className="break-words font-display text-[1.5rem] leading-tight text-foreground tabular-nums sm:text-[1.75rem]">{value ?? 0}</strong>
      {detail && <small className="truncate text-xs leading-5 text-muted-foreground">{detail}</small>}
    </div>
  );
}

export function StatusPill({ status, severity }) {
  const normalized = (status || severity || "unknown").toLowerCase();
  const tone =
    normalized === "ready" || normalized === "ok" || normalized === "info" ||
    normalized === "verified" || normalized === "passed" || normalized === "approved" ||
    normalized === "closed" || normalized === "active"
      ? "good"
      : normalized === "degraded" || normalized === "warn" || normalized === "warning" ||
        normalized === "verifying" || normalized === "pending" || normalized === "paused" ||
        normalized === "stopped" || normalized === "needs improvement" || normalized === "medium"
        ? "warn"
      : normalized === "failed" || normalized === "error" || normalized === "critical" || normalized === "high"
        ? "bad"
        : "neutral";
  const variant = tone === "good" ? "success" : tone === "warn" ? "warning" : tone === "bad" ? "destructive" : "muted";
  const Icon = tone === "good" ? CircleCheck : tone === "warn" ? CircleAlert : tone === "bad" ? CircleX : CircleDashed;
  return (
    <Badge variant={variant}>
      <Icon aria-hidden="true" />
      {status || severity || "unknown"}
    </Badge>
  );
}

export function LoadingState({ label = "Loading" }) {
  return (
    <div className="flex min-h-32 items-center justify-center gap-2.5 p-6 text-sm text-muted-foreground">
      <Loader2 aria-hidden="true" className="size-4 animate-spin text-brand" />
      <span className="font-mono text-xs uppercase tracking-[0.1em]">{label}</span>
    </div>
  );
}

// EmptyState says there is nothing here and, when there is one, what to do
// next. Its icon defaults to the view's own.
export function EmptyState(props) {
  const inRouter = useInRouterContext();
  return inRouter && !props.icon ? <RoutedEmptyState {...props} /> : <EmptyStateView {...props} />;
}

function RoutedEmptyState(props) {
  const route = routeDefinition(useLocation().pathname);
  return <EmptyStateView {...props} icon={route?.icon} />;
}

function EmptyStateView({ title = "Nothing here", message, children, icon, tone }) {
  return (
    <div data-tone={tone} className="flex items-start gap-3 py-5 text-sm">
      <span aria-hidden="true" className="tone-tile pt-0.5">
        {React.createElement(icon || Sparkle, { className: "size-[1.125rem]", strokeWidth: 1.75 })}
      </span>
      <div className="min-w-0 max-w-xl">
        <strong className="font-bold text-foreground">{title}</strong>
        {message && <p className="mt-0.5 leading-6 text-muted-foreground">{message}</p>}
        {children ? <div className="mt-3">{children}</div> : null}
      </div>
    </div>
  );
}

export function ErrorState({ title = "Unavailable", message }) {
  return (
    <Alert variant="destructive">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  );
}

export function PermissionState({ title = "No access", message }) {
  return (
    <Alert variant="warning">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  );
}

export function DataErrorState({
  error,
  permissionMessage = "Your role can't see this.",
  unavailableMessage = "Couldn't reach the server.",
  queryMessage,
}) {
  if (error?.kind === "auth") {
    return <PermissionState message={permissionMessage} />;
  }
  if (error?.kind === "unavailable") {
    return <ErrorState title="Unavailable" message={unavailableMessage} />;
  }
  return <ErrorState title={error?.kind === "graphql" ? "Query failed" : "Unavailable"} message={queryMessage || error?.message || unavailableMessage} />;
}
