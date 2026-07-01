import { useEffect, useMemo, useState } from "react";
import {
  IconSearch,
  IconUnlink,
  IconDownload,
} from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Empty, EmptyHeader, EmptyDescription, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader, Mono } from "@/components/PageHeader";
import { cn, fuzzyMatch } from "@/lib/utils";
import {
  listAgentSkills,
  removeAgentSkill,
  importSkill,
  type AgentInfo,
  type AgentSkill,
} from "@/lib/api";

const SOURCE_LABEL: Record<AgentSkill["source"], string> = {
  center: "技能中心",
  real: "真实目录",
  external: "外部软链接",
};

export function AgentSkills({ agents }: { agents: AgentInfo[] }) {
  const [agentId, setAgentId] = useState<string | null>(null);
  const [skills, setSkills] = useState<AgentSkill[] | null>(null);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  // Default to the first agent once the list loads.
  useEffect(() => {
    if (!agentId && agents.length) setAgentId(agents[0].id);
  }, [agents, agentId]);

  async function refresh(id: string) {
    setSkills(null);
    try {
      setSkills(await listAgentSkills(id));
    } catch (e) {
      toast.error(String(e));
      setSkills([]);
    }
  }
  useEffect(() => {
    if (agentId) refresh(agentId);
  }, [agentId]);

  const current = agents.find((a) => a.id === agentId);

  const filtered = useMemo(() => {
    if (!skills) return [];
    const q = query.trim();
    return q ? skills.filter((s) => fuzzyMatch(q, s.name)) : skills;
  }, [skills, query]);

  async function onRemove(name: string) {
    if (!agentId) return;
    setBusy(name);
    try {
      await removeAgentSkill(agentId, name);
      toast.success(`已从 ${current?.name} 移除 ${name}`);
      await refresh(agentId);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function onImport(name: string) {
    if (!agentId) return;
    setBusy(name);
    try {
      await importSkill(agentId, name);
      toast.success(`已导入 ${name} 到技能中心`);
      await refresh(agentId);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <PageHeader title="Agent 技能">
        查看某个 agent 已拥有的技能，分配或移除
        {current && (
          <>
            {" "}· 当前：<Mono>{current.name}</Mono>
          </>
        )}
      </PageHeader>

      <div className="flex flex-wrap items-center gap-2.5 px-6 py-3.5">
        <div className="relative min-w-[200px] flex-1">
          <IconSearch className="text-muted-foreground absolute left-3 top-1/2 size-[15px] -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="在该 agent 中搜索技能…"
            className="pl-9"
          />
        </div>
      </div>

      {/* agent selector */}
      <div className="flex flex-wrap gap-1.5 px-6 pb-1">
        {agents.map((a) => (
          <button
            key={a.id}
            onClick={() => setAgentId(a.id)}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12.5px] font-medium transition-colors",
              agentId === a.id
                ? "bg-primary text-primary-foreground border-transparent"
                : "border-border bg-muted hover:bg-foreground/5",
            )}
          >
            <span className="size-2 rounded-full" style={{ background: a.color }} />
            {a.name}
          </button>
        ))}
      </div>

      <div className="text-muted-foreground px-6 pb-1 pt-2 text-[11.5px]">
        来源标签：<b className="text-primary">技能中心</b> 软链接可移除；
        <b className="text-emerald-600">真实目录</b> 与 <b>外部软链接</b> 受保护、不会被删除，可导入到技能中心统一管理
      </div>

      <div className="grid flex-1 content-start gap-3 overflow-auto px-4 pb-6 pt-2 [grid-template-columns:repeat(auto-fill,minmax(360px,1fr))]">
        {skills === null ? (
          Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-[132px] rounded-[10px]" />
          ))
        ) : filtered.length === 0 ? (
          <div className="col-span-full">
            <Empty>
              <EmptyHeader>
                <EmptyTitle>该 agent 暂无技能</EmptyTitle>
                <EmptyDescription>
                  在「技能中心」点击 agent 徽标即可把技能分配到这里。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </div>
        ) : (
          filtered.map((s) => (
            <div
              key={s.name}
              className="border-border hover:border-primary/30 flex flex-col rounded-[10px] border p-3.5 transition-colors"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-[13px] font-semibold">{s.name}</span>
                <Badge
                  variant="outline"
                  className={cn(
                    "text-[10px]",
                    s.source === "center" && "border-primary/30 bg-primary/10 text-primary",
                    s.source === "real" && "border-emerald-500/30 bg-emerald-500/10 text-emerald-600",
                    s.source === "external" && "text-muted-foreground",
                  )}
                >
                  {SOURCE_LABEL[s.source]}
                </Badge>
              </div>

              <p className="text-muted-foreground mt-1.5 line-clamp-2 text-[12.5px]">
                {s.description || "—"}
              </p>
              <p className="text-muted-foreground mt-2 break-all font-mono text-[11px]">
                {s.source === "center" ? "→ " : ""}
                {s.target}
              </p>

              <div className="mt-auto flex items-center gap-1.5 pt-3">
                <div className="flex-1" />
                {s.source === "center" ? (
                  <Button
                    variant="outline"
                    size="sm"
                    className="hover:border-destructive hover:text-destructive"
                    disabled={busy === s.name}
                    onClick={() => onRemove(s.name)}
                  >
                    <IconUnlink data-icon="inline-start" />
                    移除
                  </Button>
                ) : (
                  <Button size="sm" disabled={busy === s.name} onClick={() => onImport(s.name)}>
                    <IconDownload data-icon="inline-start" />
                    导入
                  </Button>
                )}
              </div>
            </div>
          ))
        )}
      </div>
    </>
  );
}
