export default function DaybreakBadge({ supported }: { supported?: boolean }) {
  if (!supported) return null;
  return (
    <span className="inline-flex shrink-0 items-center rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-700 ring-1 ring-inset ring-emerald-600/20 dark:text-emerald-400 dark:ring-emerald-400/20">
      Daybreak
    </span>
  );
}
