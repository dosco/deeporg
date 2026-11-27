import * as React from "react";
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden border-0 px-0 py-px font-mono text-[0.6875rem] font-medium leading-[1.125rem] whitespace-nowrap transition-colors [&_svg]:size-3",
  {
    variants: {
      variant: {
        default: "text-brand",
        info: "text-info",
        secondary: "text-secondary-foreground",
        destructive: "text-destructive",
        outline: "px-0 text-muted-foreground",
        success: "text-success",
        warning: "text-warning",
        muted: "text-muted-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

function Badge({ className, variant, ...props }) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { Badge, badgeVariants };
