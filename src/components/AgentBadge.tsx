import { cn } from "@/lib/utils";
import type { AgentInfo } from "@/lib/api";

/** Clickable agent pill — lit when the skill/pack is linked into that agent. */
export function AgentBadge({
  agent,
  active,
  onToggle,
  busy,
}: {
  agent: AgentInfo;
  active: boolean;
  onToggle: () => void;
  busy?: boolean;
}) {
  return (
    <button
      type="button"
      disabled={busy}
      onClick={onToggle}
      title={active ? `从 ${agent.name} 移除` : `分配给 ${agent.name}`}
      className={cn(
        "group inline-flex select-none items-center gap-1.5 rounded-full border px-2.5 py-[3px] text-[10.5px] font-semibold transition-colors disabled:opacity-50",
        active
          ? "border-primary/30 bg-primary/10 text-foreground hover:border-destructive/50 hover:bg-destructive/10 hover:text-destructive"
          : "border-border text-muted-foreground opacity-70 hover:border-primary/30 hover:bg-primary/10 hover:opacity-100",
      )}
    >
      <span
        className="size-1.5 shrink-0 rounded-full"
        style={{ background: agent.color }}
      />
      {agent.name}
    </button>
  );
}
