import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { InRail, RailSlot, RailSlotProvider } from "./rail-slot";

describe("rail slot", () => {
  it("puts a page's list in the menu while the menu offers room", () => {
    const { rerender } = render(
      <RailSlotProvider>
        <aside aria-label="Menu"><RailSlot /></aside>
        <main><InRail><p>Revenue question</p></InRail></main>
      </RailSlotProvider>,
    );
    expect(screen.getByRole("complementary", { name: "Menu" })).toHaveTextContent("Revenue question");

    rerender(
      <RailSlotProvider>
        <aside aria-label="Menu" />
        <main><InRail><p>Revenue question</p></InRail></main>
      </RailSlotProvider>,
    );
    expect(screen.queryByText("Revenue question")).toBeNull();
  });
});
