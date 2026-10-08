import * as React from "react";

import { cn } from "@/lib/utils";

function Table({ className, ...props }: React.ComponentProps<"table">) {
  const scrollRef = React.useRef<HTMLDivElement>(null);
  const scrollbarRef = React.useRef<HTMLDivElement>(null);
  const dragRef = React.useRef<{ pointerId: number; offset: number } | null>(
    null,
  );
  const id = React.useId();
  const [scrollState, setScrollState] = React.useState({
    clientWidth: 0,
    scrollLeft: 0,
    scrollWidth: 0,
  });

  React.useEffect(() => {
    const scroll = scrollRef.current;
    if (!scroll) return;

    const update = () => {
      setScrollState({
        clientWidth: scroll.clientWidth,
        scrollLeft: scroll.scrollLeft,
        scrollWidth: scroll.scrollWidth,
      });
    };
    const observer = new ResizeObserver(update);
    observer.observe(scroll);
    if (scroll.firstElementChild) observer.observe(scroll.firstElementChild);
    scroll.addEventListener("scroll", update, { passive: true });
    update();

    return () => {
      observer.disconnect();
      scroll.removeEventListener("scroll", update);
    };
  }, []);

  const hasHorizontalOverflow =
    scrollState.scrollWidth > scrollState.clientWidth + 1;
  const thumbWidth = hasHorizontalOverflow
    ? Math.max(6, (scrollState.clientWidth / scrollState.scrollWidth) * 100)
    : 100;
  const thumbLeft = hasHorizontalOverflow
    ? (scrollState.scrollLeft /
        (scrollState.scrollWidth - scrollState.clientWidth)) *
      (100 - thumbWidth)
    : 0;

  const handleScrollbarPointerDown = (
    event: React.PointerEvent<HTMLDivElement>,
  ) => {
    if (event.button !== 0 || !scrollRef.current || !scrollbarRef.current)
      return;
    const track = scrollbarRef.current;
    const thumb = track.querySelector<HTMLElement>(
      "[data-slot='table-scrollbar-thumb']",
    );
    if (!thumb) return;

    const trackRect = track.getBoundingClientRect();
    const thumbRect = thumb.getBoundingClientRect();
    const isThumb =
      event.target instanceof Element &&
      event.target.closest("[data-slot='table-scrollbar-thumb']");
    const offset = isThumb
      ? event.clientX - thumbRect.left
      : thumbRect.width / 2;
    dragRef.current = { pointerId: event.pointerId, offset };
    track.setPointerCapture(event.pointerId);

    if (!isThumb) {
      const travel = trackRect.width - thumbRect.width;
      const position = Math.max(
        0,
        Math.min(travel, event.clientX - trackRect.left - offset),
      );
      scrollRef.current.scrollLeft =
        (position / travel) *
        (scrollRef.current.scrollWidth - scrollRef.current.clientWidth);
    }
    event.preventDefault();
  };

  const handleScrollbarPointerMove = (
    event: React.PointerEvent<HTMLDivElement>,
  ) => {
    const drag = dragRef.current;
    const scroll = scrollRef.current;
    const track = scrollbarRef.current;
    if (!drag || drag.pointerId !== event.pointerId || !scroll || !track)
      return;

    const thumb = track.querySelector<HTMLElement>(
      "[data-slot='table-scrollbar-thumb']",
    );
    if (!thumb) return;
    const trackRect = track.getBoundingClientRect();
    const travel = trackRect.width - thumb.getBoundingClientRect().width;
    if (travel <= 0) return;
    const position = Math.max(
      0,
      Math.min(travel, event.clientX - trackRect.left - drag.offset),
    );
    scroll.scrollLeft =
      (position / travel) * (scroll.scrollWidth - scroll.clientWidth);
  };

  const handleScrollbarKeyDown = (
    event: React.KeyboardEvent<HTMLDivElement>,
  ) => {
    const scroll = scrollRef.current;
    if (!scroll) return;
    const page = scroll.clientWidth * 0.8;
    switch (event.key) {
      case "ArrowLeft":
        scroll.scrollBy({ left: -48 });
        break;
      case "ArrowRight":
        scroll.scrollBy({ left: 48 });
        break;
      case "PageUp":
        scroll.scrollBy({ left: -page });
        break;
      case "PageDown":
        scroll.scrollBy({ left: page });
        break;
      case "Home":
        scroll.scrollTo({ left: 0 });
        break;
      case "End":
        scroll.scrollTo({ left: scroll.scrollWidth });
        break;
      default:
        return;
    }
    event.preventDefault();
  };

  return (
    <div className="min-w-0 w-full">
      <div
        id={id}
        ref={scrollRef}
        data-slot="table-container"
        className="thin-scrollbar relative w-full overflow-x-auto"
      >
        <table
          data-slot="table"
          className={cn("w-full caption-bottom text-sm", className)}
          {...props}
        />
      </div>
      {hasHorizontalOverflow ? (
        <div
          ref={scrollbarRef}
          data-slot="table-horizontal-scrollbar"
          role="scrollbar"
          aria-label="Horizontal table scroll"
          aria-controls={id}
          aria-orientation="horizontal"
          aria-valuemin={0}
          aria-valuemax={scrollState.scrollWidth - scrollState.clientWidth}
          aria-valuenow={Math.round(scrollState.scrollLeft)}
          tabIndex={0}
          onPointerDown={handleScrollbarPointerDown}
          onPointerMove={handleScrollbarPointerMove}
          onPointerUp={() => {
            dragRef.current = null;
          }}
          onPointerCancel={() => {
            dragRef.current = null;
          }}
          onKeyDown={handleScrollbarKeyDown}
          className="table-horizontal-scrollbar"
        >
          <span
            data-slot="table-scrollbar-thumb"
            className="table-horizontal-scrollbar__thumb"
            style={{ left: `${thumbLeft}%`, width: `${thumbWidth}%` }}
          />
        </div>
      ) : null}
    </div>
  );
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("[&_tr]:border-b", className)}
      {...props}
    />
  );
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn("[&_tr:last-child]:border-0", className)}
      {...props}
    />
  );
}

function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        "border-t bg-muted/50 font-medium [&>tr]:last:border-b-0",
        className,
      )}
      {...props}
    />
  );
}

function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        "border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted",
        className,
      )}
      {...props}
    />
  );
}

function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "h-9 px-3 text-left align-middle text-xs font-semibold uppercase whitespace-nowrap text-muted-foreground [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
        className,
      )}
      {...props}
    />
  );
}

function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  return (
    <td
      data-slot="table-cell"
      className={cn(
        "px-3 py-2.5 align-middle text-sm whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
        className,
      )}
      {...props}
    />
  );
}

function TableCaption({
  className,
  ...props
}: React.ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("mt-4 text-sm text-muted-foreground", className)}
      {...props}
    />
  );
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption,
};
