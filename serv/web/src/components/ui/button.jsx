import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-semibold tracking-tight transition-[background-color,color,box-shadow,border-color,filter,transform] duration-150 outline-none focus-visible:ring-ring/40 focus-visible:ring-[3px] disabled:pointer-events-none disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "rounded-full bg-[linear-gradient(180deg,color-mix(in_srgb,var(--primary)_78%,#fff)_0%,var(--primary)_100%)] font-semibold text-primary-foreground shadow-[inset_0_1px_0_rgba(255,255,255,0.6),0_10px_26px_-12px_var(--glow)] hover:-translate-y-px",
        contrast: "bg-foreground font-bold text-background hover:bg-foreground/88",
        destructive: "bg-destructive text-white hover:bg-destructive/88",
        outline: "rounded-full border border-input bg-transparent hover:border-brand hover:text-accent-foreground",
        secondary: "rounded-full border border-input bg-card text-secondary-foreground hover:border-brand",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        link: "text-brand underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-4 py-2",
        sm: "h-8 px-3.5 text-[0.8125rem]",
        lg: "h-11 px-7 text-[0.9375rem]",
        icon: "size-9",
        "icon-sm": "size-8",
        "icon-xs": "size-7",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

const Button = React.forwardRef(({ className, variant, size, asChild = false, ...props }, ref) => {
  const Comp = asChild ? Slot : "button";
  return <Comp ref={ref} className={cn(buttonVariants({ variant, size, className }))} {...props} />;
});
Button.displayName = "Button";

export { Button, buttonVariants };
