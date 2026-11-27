import React from "react";

import { cn } from "@/lib/utils";

// BrandMark is the deeporg.io mark: a sea-glass orb lit from the top left,
// with a soft halo. The site draws it before the wordmark in its header; the
// favicon is the same orb on a dark ink tile.
export function BrandMark({ className, large = false }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "inline-block shrink-0 rounded-full bg-[radial-gradient(circle_at_34%_30%,#fff_0,var(--sea)_46%,var(--brand)_100%)]",
        large
          ? "size-9 shadow-[0_0_0_9px_color-mix(in_srgb,var(--sea)_18%,transparent),0_0_40px_color-mix(in_srgb,var(--sea)_30%,transparent)]"
          : "size-2.5 shadow-[0_0_0_4px_color-mix(in_srgb,var(--sea)_22%,transparent)]",
        className
      )}
    />
  );
}
