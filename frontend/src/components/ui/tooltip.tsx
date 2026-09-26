import * as React from "react"
import { Tooltip as TooltipPrimitive } from "radix-ui"

import { cn } from "@/lib/utils"

function TooltipProvider({
  delayDuration = 0,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return (
    <TooltipPrimitive.Provider
      data-slot="tooltip-provider"
      delayDuration={delayDuration}
      {...props}
    />
  )
}

function Tooltip({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  return <TooltipPrimitive.Root data-slot="tooltip" {...props} />
}

function TooltipTrigger({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Trigger>) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />
}

type ResponsiveTooltipProps = {
  trigger: React.ReactElement
  children: React.ReactNode
  side?: React.ComponentProps<typeof TooltipPrimitive.Content>["side"]
  sideOffset?: number
  className?: string
}

// Tooltips are hover-oriented on desktop. Touch pointers need an explicit
// click toggle because Radix intentionally skips touch hover events.
function ResponsiveTooltip({ trigger, children, side, sideOffset, className }: ResponsiveTooltipProps) {
  const [open, setOpen] = React.useState(false)
  const touchTriggerRef = React.useRef(false)

  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger
        asChild
        onPointerDown={(event) => {
          if (event.pointerType !== 'touch') return
          event.preventDefault()
          touchTriggerRef.current = true
          setOpen((current) => !current)
        }}
        onPointerLeave={(event) => {
          if (event.pointerType === 'touch') event.preventDefault()
        }}
        onClick={(event) => {
          if (!touchTriggerRef.current) return
          event.preventDefault()
          touchTriggerRef.current = false
        }}
      >
        {trigger}
      </TooltipTrigger>
      <TooltipContent side={side} sideOffset={sideOffset} className={className}>
        {children}
      </TooltipContent>
    </Tooltip>
  )
}

function TooltipContent({
  className,
  arrowClassName,
  sideOffset = 0,
  children,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content> & {
  arrowClassName?: string
}) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn(
          "z-50 w-fit origin-(--radix-tooltip-content-transform-origin) animate-in rounded-md bg-foreground px-3 py-1.5 text-xs text-balance text-background fade-in-0 zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95",
          className
        )}
        {...props}
      >
        {children}
        <TooltipPrimitive.Arrow className={cn("z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px] bg-foreground fill-foreground", arrowClassName)} />
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  )
}

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider, ResponsiveTooltip }
