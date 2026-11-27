import * as React from "react";
import { createPortal } from "react-dom";

// The left menu has room under its views for a page's own list, such as the
// agent's chats. The menu offers the space only while it is open on a wide
// screen; a page with nowhere to put its list shows a button instead.
const RailSlotContext = React.createContext({ slot: null, setSlot: () => {} });

export function RailSlotProvider({ children }) {
  const [slot, setSlot] = React.useState(null);
  const value = React.useMemo(() => ({ slot, setSlot }), [slot]);
  return <RailSlotContext.Provider value={value}>{children}</RailSlotContext.Provider>;
}

// useRailSlot returns the menu's space for a page list, or null when the menu
// has none to offer.
export function useRailSlot() {
  return React.useContext(RailSlotContext).slot;
}

export function RailSlot({ className }) {
  const { setSlot } = React.useContext(RailSlotContext);
  return <div ref={setSlot} className={className} />;
}

export function InRail({ children }) {
  const slot = useRailSlot();
  return slot ? createPortal(children, slot) : null;
}
