import React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { LogIn, Monitor, Moon, PanelLeftClose, PanelLeftOpen, Search, Sun } from "lucide-react";

import { availableWorkspaces, commandItems, visibleItems, workspaceDefinitions, workspaceIDFromPath } from "@/app/workspaces";
import { listNotifications, notificationCountKey } from "@/services/notifications";
import { cn } from "@/lib/utils";
import {
  clearOperatorIdentity,
  hasOperatorIdentity,
  useOperatorIdentity,
  writeOperatorIdentity,
} from "@/services/identity";
import { Button } from "@/components/ui/button";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { useTheme } from "@/components/theme-provider";
import { signInState } from "@/services/session";
import { useConsole } from "./console-context";
import { RailSlot } from "./rail-slot";
import SignInPanel from "./SignInPanel";
import { BrandMark } from "@/components/brand-mark";

const lastRouteKey = (workspaceID) => `graphjin-console-last-route:${workspaceID}`;

// The rail can fold to icons on wide screens. The choice is kept per browser.
export const RAIL_COLLAPSED_KEY = "deeporg-console-rail-collapsed";

function readRailCollapsed() {
  try {
    return localStorage.getItem(RAIL_COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

// RailTip names an icon when the rail is folded and its label is hidden.
function RailTip({ collapsed, label, children }) {
  if (!collapsed) {
    return children;
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side="right" sideOffset={10} className="hidden md:block">{label}</TooltipContent>
    </Tooltip>
  );
}

// The menu is a side rail from the md breakpoint up; below it, a bottom bar.
function useWideScreen() {
  const query = "(min-width: 768px)";
  const [wide, setWide] = React.useState(() => window.matchMedia?.(query).matches ?? false);
  React.useEffect(() => {
    const media = window.matchMedia?.(query);
    if (!media) return undefined;
    const onChange = (event) => setWide(event.matches);
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, []);
  return wide;
}

const Header = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const { bootstrap } = useConsole();
  const { theme, setTheme } = useTheme();
  const identity = useOperatorIdentity();
  const [identityOpen, setIdentityOpen] = React.useState(false);
  const [commandOpen, setCommandOpen] = React.useState(false);
  const [collapsed, setCollapsed] = React.useState(readRailCollapsed);
  const wide = useWideScreen();
  const [draft, setDraft] = React.useState(identity);
  const workspaceID = workspaceIDFromPath(location.pathname);
  const workspace = workspaceDefinitions[workspaceID];
  const workspaces = availableWorkspaces(bootstrap);
  // A workspace reached by URL still gets its tab, so the nav always shows where you are.
  const workspaceTabs = workspace && !workspaces.some((item) => item.id === workspace.id) ? [...workspaces, workspace] : workspaces;
  const scopeLabel = bootstrap?.scope?.environment || bootstrap?.scope?.source || "local";
  const session = signInState(bootstrap);
  const promptedSignIn = React.useRef(false);
  const navItems = visibleItems(workspace, bootstrap);
  const notificationsOn = navItems.some((item) => item.count === "notifications");
  // The stream (useNotificationStream) keeps this current; the query only
  // seeds it on first load.
  const { data: unseenCount = 0 } = useQuery({
    queryKey: notificationCountKey,
    queryFn: () => listNotifications({ unseen: true, limit: 1 }).then((payload) => payload?.unseen_count ?? 0),
    enabled: notificationsOn,
    staleTime: Infinity,
  });

  React.useEffect(() => {
    if (workspaceDefinitions[workspaceID]) {
      localStorage.setItem(lastRouteKey(workspaceID), `${location.pathname}${location.search}`);
    }
  }, [location.pathname, location.search, workspaceID]);

  // On phones the view list is a scrolling bottom bar; keep the current view in it.
  const viewNavRef = React.useRef(null);
  React.useEffect(() => {
    viewNavRef.current?.querySelector("[aria-current=page]")?.scrollIntoView?.({ block: "nearest", inline: "center" });
  }, [location.pathname]);

  React.useEffect(() => {
    if (!identityOpen) {
      setDraft(identity);
    }
  }, [identity, identityOpen]);

  // A JWT-only server shows nothing until the console has a session, so ask
  // once instead of leaving every page to fail with 401.
  React.useEffect(() => {
    if (session.required && !session.signedIn && !promptedSignIn.current) {
      promptedSignIn.current = true;
      setIdentityOpen(true);
    }
  }, [session.required, session.signedIn]);

  React.useEffect(() => {
    const handleKeyDown = (event) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen((open) => !open);
      }
      if ((event.metaKey || event.ctrlKey) && event.key === "\\") {
        event.preventDefault();
        toggleRail();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  function toggleRail() {
    setCollapsed((current) => {
      const next = !current;
      try {
        localStorage.setItem(RAIL_COLLAPSED_KEY, next ? "1" : "0");
      } catch {
        // Storage can be refused; the rail still folds for this page.
      }
      return next;
    });
  }

  function switchWorkspace(target) {
    const stored = localStorage.getItem(lastRouteKey(target.id));
    navigate(stored?.startsWith(`/${target.id}/`) ? stored : target.defaultPath);
  }

  function selectCommand(path) {
    navigate(path);
    setCommandOpen(false);
  }

  function saveIdentity(event) {
    event.preventDefault();
    if (!draft.userId.trim()) {
      return;
    }
    writeOperatorIdentity(draft);
    void queryClient.invalidateQueries();
    setIdentityOpen(false);
  }

  function sessionChanged() {
    // Every cached query belongs to the previous identity.
    queryClient.clear();
    window.location.reload();
  }

  function clearIdentity() {
    clearOperatorIdentity();
    void queryClient.invalidateQueries();
    setIdentityOpen(false);
  }

  const identityLabel = session.required
    ? (session.signedIn ? session.name : "Sign in")
    : (identity.userId || bootstrap?.identity?.role || "Identity");
  const identityDetail = [
    session.required ? (session.signedIn ? session.role || bootstrap?.identity?.role : "signed out") : (identity.role || bootstrap?.identity?.role),
    scopeLabel,
  ].filter(Boolean).join(" · ");

  const themeOptions = [
    { value: "dark", label: "Dark", icon: Moon },
    { value: "light", label: "Light", icon: Sun },
    { value: "system", label: "System", icon: Monitor },
  ];

  // One rail serves every width: a top bar with a bottom tab bar on phones,
  // and a labelled rail from md up. Each view keeps its own icon and colour,
  // so the rail, the search palette and the page header agree.
  const cycleTheme = () => {
    const order = themeOptions.map((option) => option.value);
    setTheme(order[(order.indexOf(theme) + 1) % order.length]);
  };
  const currentTheme = themeOptions.find((option) => option.value === theme) || themeOptions[0];
  const hideWhenFolded = collapsed ? "md:sr-only" : "";

  return (
    <TooltipProvider delayDuration={150}>
    <header
      data-collapsed={collapsed ? "true" : undefined}
      className={cn(
        "sticky top-0 z-40 flex h-14 shrink-0 items-center gap-3 px-4 transition-[width] duration-200 md:h-dvh md:flex-col md:items-stretch md:gap-0 md:self-start md:py-5",
        collapsed ? "md:w-16 md:px-3" : "md:w-60 md:px-4 xl:w-64 xl:px-5"
      )}
    >
      <div aria-hidden="true" className="absolute inset-0 -z-10 bg-background/85 backdrop-blur-xl md:hidden" />
      <div aria-hidden="true" className="absolute inset-y-0 right-0 -z-10 hidden w-px bg-gradient-to-b from-transparent via-border to-transparent md:block" />

      <Link
        to={workspace?.defaultPath || "/user/agent"}
        className={cn("group flex h-8 shrink-0 items-center gap-2.5", collapsed ? "md:justify-center" : "md:px-2.5")}
        aria-label="DeepORG console home"
      >
        <BrandMark />
        <span className={cn("hidden font-display text-[1.18rem] font-bold leading-none tracking-[-0.035em] min-[420px]:inline", collapsed && "md:hidden")}>DeepORG</span>
      </Link>

      <div role="group" aria-label="Workspace" className={cn("flex items-center gap-4 md:mt-8", collapsed ? "md:flex-col md:gap-3" : "md:px-1.5")}>
          {workspaceTabs.map((item) => {
            const current = item.id === workspaceID;
            const Icon = item.icon;
            return (
              <RailTip key={item.id} collapsed={collapsed} label={item.label}>
              <button
                type="button"
                data-tone={item.tone}
                aria-current={current ? "page" : undefined}
                title={collapsed ? undefined : item.description}
                onClick={() => switchWorkspace(item)}
                className={cn(
                  "relative flex items-center gap-1.5 py-1 text-xs font-semibold transition-colors",
                  "after:absolute after:inset-x-0 after:-bottom-0.5 after:h-px after:bg-transparent",
                  current ? "text-foreground after:bg-brand" : "text-muted-foreground hover:text-foreground"
                )}
              >
                {Icon && <Icon aria-hidden="true" className={cn(collapsed ? "size-3.5 md:size-4" : "size-3.5", current && "text-[var(--tone)]")} />}
                <span className={hideWhenFolded}>{item.label}</span>
              </button>
              </RailTip>
            );
          })}
      </div>

      <nav
        ref={viewNavRef}
        className="scrollbar-none fixed inset-x-0 bottom-0 z-40 flex h-[var(--chrome-bottom)] items-stretch overflow-x-auto border-t bg-background/90 px-1 pb-[env(safe-area-inset-bottom)] backdrop-blur-xl md:static md:mt-6 md:h-auto md:flex-col md:gap-0.5 md:overflow-visible md:border-0 md:bg-transparent md:p-0 md:backdrop-blur-none"
        aria-label={`${workspace?.label || "Console"} workspace`}
      >
        {navItems.map((item) => {
          const active = location.pathname.startsWith(item.path);
          const Icon = item.icon;
          const showCount = item.count === "notifications" && unseenCount > 0;
          return (
            <RailTip key={item.path} collapsed={collapsed} label={showCount ? `${item.label} · ${unseenCount} unseen` : item.label}>
            <Link
              to={item.path}
              data-tone={item.tone}
              aria-current={active ? "page" : undefined}
              className={cn(
                "group relative flex min-w-16 flex-1 shrink-0 flex-col items-center justify-center gap-1 px-2 text-[0.6875rem] font-semibold text-muted-foreground transition-colors hover:text-foreground",
                "md:min-w-0 md:flex-none md:flex-row md:justify-start md:gap-3 md:px-1.5 md:py-1.5 md:text-[0.9375rem]",
                "before:absolute before:inset-x-5 before:top-0 before:h-0.5 before:rounded-full before:bg-transparent before:transition-colors",
                "md:before:inset-x-auto md:before:inset-y-2 md:before:h-auto md:before:w-[3px]",
                collapsed ? "md:justify-center md:px-0 md:before:-left-3" : "md:before:-left-4 xl:before:-left-5",
                active && "text-foreground before:bg-[var(--tone)] md:font-bold"
              )}
            >
              <span
                aria-hidden="true"
                className={cn(
                  "relative grid size-6 shrink-0 place-items-center text-[var(--tone)] transition-opacity",
                  active ? "opacity-100" : "opacity-70 group-hover:opacity-100"
                )}
              >
                {Icon && <Icon className="size-[1.0625rem]" strokeWidth={1.9} />}
                {showCount && <span className={cn("absolute -right-1 -top-1 size-2 rounded-full bg-brand ring-2 ring-background", !collapsed && "md:hidden")} />}
              </span>
              <span className={cn("truncate", hideWhenFolded)}>{item.label}</span>
              {showCount && (
                <span className={cn("ml-auto hidden min-w-5 rounded-full bg-accent px-1.5 text-center font-mono text-[0.6875rem] font-semibold leading-5 text-foreground", collapsed ? "sr-only" : "md:inline")} aria-label={`${unseenCount} unseen`}>{unseenCount}</span>
              )}
            </Link>
            </RailTip>
          );
        })}
      </nav>

      {!collapsed && wide && <RailSlot className="-mr-2 mb-5 mt-7 hidden min-h-0 flex-1 overflow-y-auto pr-2 md:block" />}

      <div className={cn("ml-auto flex items-center gap-1 md:ml-0 md:mt-auto md:flex-col md:gap-3", collapsed ? "md:items-center" : "md:items-stretch")}>
        <RailTip collapsed={collapsed} label="Search · ⌘K">
        <button
          type="button"
          onClick={() => setCommandOpen(true)}
          className={cn("flex items-center gap-2 p-2 text-sm text-muted-foreground transition-colors hover:text-foreground md:py-1", collapsed ? "md:px-1" : "md:px-1.5")}
          aria-label="Search console"
        >
          <Search className="size-4" aria-hidden="true" />
          <span className={cn("hidden md:inline", collapsed && "md:hidden")}>Search</span>
          <kbd aria-hidden="true" className={cn("ml-auto hidden font-mono text-[0.6875rem] md:inline", collapsed && "md:hidden")}>⌘K</kbd>
        </button>
        </RailTip>

        <RailTip collapsed={collapsed} label={collapsed ? "Expand menu · ⌘\\" : ""}>
        <button
          type="button"
          onClick={toggleRail}
          aria-label={collapsed ? "Expand menu" : "Collapse menu"}
          aria-expanded={!collapsed}
          className={cn("hidden items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground md:flex md:py-1", collapsed ? "md:px-1" : "md:px-1.5")}
        >
          {collapsed ? <PanelLeftOpen aria-hidden="true" className="size-4" /> : <PanelLeftClose aria-hidden="true" className="size-4" />}
          <span className={cn(collapsed && "md:hidden")}>Collapse</span>
          <kbd aria-hidden="true" className={cn("ml-auto font-mono text-[0.6875rem]", collapsed && "md:hidden")}>{"⌘\\"}</kbd>
        </button>
        </RailTip>

        {collapsed && (
          <RailTip collapsed label={`${currentTheme.label} theme`}>
            <button type="button" onClick={cycleTheme} aria-label={`Theme: ${currentTheme.label}. Change theme`} className="hidden py-1 text-brand transition-colors hover:text-foreground md:flex">
              <currentTheme.icon aria-hidden="true" className="size-4" />
            </button>
          </RailTip>
        )}

        <div role="radiogroup" aria-label="Appearance" className={cn("hidden items-center gap-3 md:px-1.5", !collapsed && "md:flex")}>
          {themeOptions.map(({ value, label, icon: Icon }) => (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={theme === value}
              aria-label={`${label} theme`}
              title={`${label} theme`}
              onClick={() => setTheme(value)}
              className={cn(
                "flex items-center justify-center py-1 transition-colors",
                theme === value ? "text-brand" : "text-muted-foreground hover:text-foreground"
              )}
            >
              <Icon aria-hidden="true" className="size-3.5" />
            </button>
          ))}
        </div>

        <Sheet open={identityOpen} onOpenChange={setIdentityOpen}>
          <SheetTrigger asChild>
            <button
              type="button"
              className={cn("group flex min-w-0 items-center gap-2.5 p-1 text-left [&:hover_.identity-name]:text-brand", collapsed ? "md:justify-center md:px-0" : "md:px-1.5")}
              aria-label={session.required
                ? (session.signedIn ? `Signed in as ${session.name}` : "Sign in")
                : (hasOperatorIdentity(identity) ? `Identity: ${identity.userId}` : "Set operator identity")}
            >
              <Avatar label={session.required && !session.signedIn ? "" : identityLabel} />
              <span className={cn("hidden min-w-0", !collapsed && "md:grid")}>
                <span className="identity-name truncate text-sm font-bold transition-colors">{identityLabel}</span>
                <span className="truncate font-mono text-[0.6875rem] text-muted-foreground">{identityDetail}</span>
              </span>
            </button>
          </SheetTrigger>
          <SheetContent side="right" className="grid w-[min(92vw,26rem)] grid-rows-[auto_1fr] sm:w-[26rem]">
            <SheetHeader className="pr-8">
              <SheetTitle>{session.required ? (session.signedIn ? "Session" : "Sign in") : "Identity"}</SheetTitle>
              <SheetDescription className={session.required ? "sr-only" : undefined}>
                {session.required
                  ? "Your console session on this server."
                  : "Sent as headers in development to scope your data."}
              </SheetDescription>
            </SheetHeader>
            {session.required ? <SignInPanel state={session} onChange={sessionChanged} /> : <form className="grid content-start gap-4 overflow-y-auto py-5" onSubmit={saveIdentity}>
              <label className="grid gap-1.5 text-sm"><span className="font-medium">User ID</span><Input value={draft.userId} onChange={(event) => setDraft((current) => ({ ...current, userId: event.target.value }))} placeholder="operator-1" autoComplete="off" /></label>
              <label className="grid gap-1.5 text-sm"><span className="font-medium">Role</span><Select value={draft.role} onChange={(event) => setDraft((current) => ({ ...current, role: event.target.value }))}><option value="">Default role</option><option value="user">User</option><option value="operator">Operator</option><option value="admin">Admin</option></Select></label>
              <label className="grid gap-1.5 text-sm"><span className="font-medium">Account ID</span><Input value={draft.accountId} onChange={(event) => setDraft((current) => ({ ...current, accountId: event.target.value }))} placeholder="account-1" autoComplete="off" /></label>
              <div className="flex items-center justify-between gap-3"><Button type="button" variant="ghost" onClick={clearIdentity} disabled={!hasOperatorIdentity(identity)}>Clear</Button><Button type="submit" disabled={!draft.userId.trim()}>Use identity</Button></div>
            </form>}
            {/* Phones have no rail footer, so appearance lives in the session sheet there. */}
            <div role="radiogroup" aria-label="Appearance" className="flex items-center gap-1 self-end border-t pt-4 md:hidden">
              {themeOptions.map(({ value, label, icon: Icon }) => (
                <button key={value} type="button" role="radio" aria-checked={theme === value} onClick={() => setTheme(value)} className={cn("flex items-center gap-1.5 px-2 py-1.5 text-xs font-medium", theme === value ? "text-foreground underline decoration-brand underline-offset-4" : "text-muted-foreground")}>
                  <Icon aria-hidden="true" className="size-3.5" />
                  {label}
                </button>
              ))}
            </div>
          </SheetContent>
        </Sheet>
      </div>

      <CommandDialog open={commandOpen} onOpenChange={setCommandOpen}>
        <CommandInput placeholder="Go to a view or run an action" autoFocus />
        <CommandList>
          <CommandEmpty>No matches</CommandEmpty>
          {workspaces.map((item) => (
            <CommandGroup key={item.id} heading={item.label}>
              {commandItems(bootstrap).filter((command) => command.workspace.id === item.id).map((command) => {
                const Icon = command.icon;
                return (
                  <CommandItem key={command.path} value={`${item.label} ${command.label}`} onSelect={() => selectCommand(command.path)}>
                    {Icon && <span data-tone={command.tone} className="tone-tile size-6"><Icon aria-hidden="true" className="size-3.5" /></span>}
                    <span>{command.label}</span>
                    {location.pathname.startsWith(command.path) && <span className="ml-auto font-mono text-[0.6875rem] text-brand">current</span>}
                  </CommandItem>
                );
              })}
            </CommandGroup>
          ))}
          <CommandGroup heading="Actions">
            <CommandItem onSelect={() => { setCommandOpen(false); setIdentityOpen(true); }}>
              <span className="tone-tile size-6" data-tone="warm"><LogIn aria-hidden="true" className="size-3.5" /></span>
              {session.required ? "Session" : "Identity"}
              <CommandShortcut>Settings</CommandShortcut>
            </CommandItem>
            <CommandItem onSelect={() => { toggleRail(); setCommandOpen(false); }}>
              <span className="tone-tile size-6" data-tone="violet">{collapsed ? <PanelLeftOpen aria-hidden="true" className="size-3.5" /> : <PanelLeftClose aria-hidden="true" className="size-3.5" />}</span>
              {collapsed ? "Expand menu" : "Collapse menu"}
              <CommandShortcut>{"⌘\\"}</CommandShortcut>
            </CommandItem>
            <CommandItem onSelect={() => { setTheme(theme === "dark" ? "light" : "dark"); setCommandOpen(false); }}>
              <span className="tone-tile size-6" data-tone="amber">{theme === "dark" ? <Sun aria-hidden="true" className="size-3.5" /> : <Moon aria-hidden="true" className="size-3.5" />}</span>
              Toggle theme
              <CommandShortcut>{theme}</CommandShortcut>
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </header>
    </TooltipProvider>
  );
};

// Avatar shows the operator's initials, or a sign-in arrow when signed out.
function Avatar({ label }) {
  const initials = String(label || "")
    .split(/[\s._@-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join("");
  return (
    <span aria-hidden="true" className="grid size-8 shrink-0 place-items-center rounded-full bg-secondary text-[0.75rem] font-semibold tracking-[0.02em] text-foreground shadow-[inset_0_0_0_1px_var(--border)]">
      {initials || <LogIn className="size-3.5" />}
    </span>
  );
}

export default Header;
