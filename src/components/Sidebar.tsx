import {
  IconLayoutGrid,
  IconPackages,
  IconRobot,
  IconSettings,
  IconServer,
  IconCpu2,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";

export type View =
  | "skills"
  | "packs"
  | "agents"
  | "models"
  | "agent-models"
  | "settings";

const NAV_GROUPS: { title: string; items: { id: View; label: string; icon: typeof IconLayoutGrid }[] }[] = [
  {
    title: "技能",
    items: [
      { id: "skills", label: "技能中心", icon: IconLayoutGrid },
      { id: "packs", label: "技能包", icon: IconPackages },
      { id: "agents", label: "Agent 技能", icon: IconRobot },
    ],
  },
  {
    title: "模型",
    items: [
      { id: "models", label: "模型中心", icon: IconServer },
      { id: "agent-models", label: "Agent 模型", icon: IconCpu2 },
    ],
  },
];

export function Sidebar({
  view,
  onChange,
  topInset = 0,
}: {
  view: View;
  onChange: (v: View) => void;
  topInset?: number;
}) {
  return (
    <aside
      style={{ paddingTop: topInset }}
      className="bg-sidebar text-sidebar-foreground flex flex-col"
    >
      {/* primary nav — grouped by section title with a solid hairline */}
      <nav className="flex flex-col gap-2.5 px-2.5 pt-2">
        {NAV_GROUPS.map((group) => (
          <div key={group.title} className="flex flex-col gap-1">
            <div className="flex flex-col gap-1.5 max-[720px]:hidden">
              <span className="text-muted-foreground/70 px-3 text-[10.5px] font-semibold uppercase tracking-wider select-none">
                {group.title}
              </span>
              <div className="bg-border mx-2 h-px" />
            </div>
            {group.items.map(({ id, label, icon: Icon }) => (
              <button
                key={id}
                onClick={() => onChange(id)}
                className={cn(
                  "font-heading flex items-center gap-3 rounded-lg px-3 py-2 text-[13.5px] font-medium transition-colors max-[720px]:justify-center max-[720px]:px-2 select-none",
                  view === id
                    ? "bg-primary text-primary-foreground"
                    : "text-foreground/65 hover:bg-foreground/[0.04] hover:text-foreground/80",
                )}
              >
                <Icon className="size-[18px] shrink-0" stroke={1.8} />
                <span className="max-[720px]:hidden">{label}</span>
              </button>
            ))}
          </div>
        ))}
      </nav>

      <div className="flex-1" />

      {/* settings — pinned at the bottom, icon only */}
      <div className="flex px-2.5 pb-3 max-[720px]:justify-center">
        <button
          onClick={() => onChange("settings")}
          title="设置"
          aria-label="设置"
          className={cn(
            "grid size-9 place-items-center rounded-lg transition-colors",
            view === "settings"
              ? "bg-foreground/[0.055] text-foreground/80"
              : "text-muted-foreground hover:bg-foreground/[0.04] hover:text-foreground/80",
          )}
        >
          <IconSettings className="size-[18px]" stroke={1.8} />
        </button>
      </div>
    </aside>
  );
}
