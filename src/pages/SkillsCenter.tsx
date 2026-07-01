import { useEffect, useMemo, useState } from "react";
import { IconSearch, IconDownload, IconEye, IconX, IconLanguage, IconRefresh } from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Empty, EmptyHeader, EmptyDescription, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { AgentBadge } from "@/components/AgentBadge";
import { ImportSkillDialog } from "@/components/ImportSkillDialog";
import { PageHeader, Mono } from "@/components/PageHeader";
import { cn, fuzzyMatch } from "@/lib/utils";
import {
  listSkills,
  assignSkill,
  unassignSkill,
  deleteSkill,
  readSkillMd,
  getSkillTranslation,
  translateSkill,
  type AgentInfo,
  type Skill,
} from "@/lib/api";

export function SkillsCenter({ agents }: { agents: AgentInfo[] }) {
  const [skills, setSkills] = useState<Skill[] | null>(null);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  const [createOpen, setCreateOpen] = useState(false);

  const [viewName, setViewName] = useState<string | null>(null);
  const [viewMd, setViewMd] = useState("");
  const [viewLang, setViewLang] = useState<"raw" | "zh">("raw");
  const [zhText, setZhText] = useState<string | null>(null);
  // idle → not checked · checking cache · missing (no cache) · loading (translating) · ready · error
  const [zhState, setZhState] = useState<
    "idle" | "checking" | "missing" | "loading" | "ready" | "error"
  >("idle");
  const [zhErr, setZhErr] = useState("");

  const [pendingDelete, setPendingDelete] = useState<string | null>(null);

  async function refresh() {
    try {
      setSkills(await listSkills());
    } catch (e) {
      toast.error(String(e));
    }
  }
  useEffect(() => {
    refresh();
  }, []);

  const filtered = useMemo(() => {
    if (!skills) return [];
    const q = query.trim();
    return q ? skills.filter((s) => fuzzyMatch(q, s.name)) : skills;
  }, [skills, query]);

  async function toggle(skill: Skill, agentId: string) {
    const key = `${skill.name}:${agentId}`;
    setBusy(key);
    const has = skill.assigned_agents.includes(agentId);
    try {
      if (has) await unassignSkill(skill.name, agentId);
      else await assignSkill(skill.name, agentId);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function onView(name: string) {
    setViewName(name);
    setViewLang("raw");
    setZhText(null);
    setZhState("idle");
    setZhErr("");
    try {
      setViewMd(await readSkillMd(name));
    } catch (e) {
      setViewMd(`# ${name}\n\n（该技能没有 SKILL.md）`);
    }
  }

  // Look up a cached translation; mark "missing" so the UI can offer to translate.
  async function checkTranslation(name: string) {
    setZhState("checking");
    setZhErr("");
    try {
      const cached = await getSkillTranslation(name);
      if (cached != null) {
        setZhText(cached);
        setZhState("ready");
      } else {
        setZhState("missing");
      }
    } catch (e) {
      setZhErr(String(e));
      setZhState("error");
    }
  }

  async function runTranslate(name: string) {
    setZhState("loading");
    setZhErr("");
    try {
      const t = await translateSkill(name);
      setZhText(t.text);
      setZhState("ready");
    } catch (e) {
      setZhErr(String(e));
      setZhState("error");
    }
  }

  function selectLang(lang: "raw" | "zh") {
    setViewLang(lang);
    if (lang === "zh" && zhState === "idle" && viewName) checkTranslation(viewName);
  }

  async function onDelete() {
    if (!pendingDelete) return;
    const name = pendingDelete;
    setPendingDelete(null);
    try {
      await deleteSkill(name);
      toast.success(`已删除 ${name}`);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    }
  }

  return (
    <>
      <PageHeader title="技能中心">
        所有技能存放于 <Mono>~/.aurora/skills/</Mono> · 通过软链接分配给 agent
      </PageHeader>

      <div className="flex flex-wrap items-center gap-2.5 px-6 py-3.5">
        <div className="relative min-w-[200px] flex-1">
          <IconSearch className="text-muted-foreground absolute left-3 top-1/2 size-[15px] -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索技能名称…"
            className="pl-9"
          />
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <IconDownload data-icon="inline-start" />
          导入技能
        </Button>
      </div>

      <div className="text-muted-foreground px-6 pb-1 text-[11.5px] select-none">
        点击技能下方的 agent 徽标即可分配 / 取消分配（创建或移除软链接）
      </div>

      <div className="grid flex-1 content-start gap-3 overflow-auto px-4 pb-6 pt-2 [grid-template-columns:repeat(auto-fill,minmax(360px,1fr))]">
        {skills === null ? (
          Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-[148px] rounded-[10px]" />
          ))
        ) : filtered.length === 0 ? (
          <div className="col-span-full">
            <Empty>
              <EmptyHeader>
                <EmptyTitle>没有技能</EmptyTitle>
                <EmptyDescription>
                  {query ? "没有匹配的技能名称。" : "点击「新建技能」创建第一个技能。"}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </div>
        ) : (
          filtered.map((skill) => (
            <div
              key={skill.name}
              className="border-border hover:border-primary/30 group relative flex flex-col rounded-[10px] border p-3.5 transition-colors"
            >
              <button
                onClick={() => setPendingDelete(skill.name)}
                title="删除技能"
                className="border-border bg-card hover:border-destructive hover:text-destructive text-muted-foreground absolute right-2.5 top-2.5 grid size-[26px] place-items-center rounded-[7px] border opacity-0 transition-opacity group-hover:opacity-100"
              >
                <IconX className="size-[14px]" />
              </button>

              <div className="flex items-center gap-2 pr-8">
                <span className="font-mono text-[13px] font-semibold">{skill.name}</span>
                {skill.assigned_agents.length > 0 && (
                  <span className="text-primary bg-primary/10 rounded px-1.5 py-px text-[10px] font-semibold">
                    已分配 {skill.assigned_agents.length}
                  </span>
                )}
              </div>

              <p className="text-muted-foreground mt-1.5 line-clamp-2 text-[12.5px]">
                {skill.description || "—"}
              </p>

              <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                {agents.map((a) => (
                  <AgentBadge
                    key={a.id}
                    agent={a}
                    active={skill.assigned_agents.includes(a.id)}
                    busy={busy === `${skill.name}:${a.id}`}
                    onToggle={() => toggle(skill, a.id)}
                  />
                ))}
              </div>

              <div className="mt-auto flex items-center gap-1.5 pt-3">
                <Button
                  variant="outline"
                  size="icon"
                  className="size-[30px]"
                  title="查看 SKILL.md"
                  onClick={() => onView(skill.name)}
                >
                  <IconEye className="size-[15px]" />
                </Button>
              </div>
            </div>
          ))
        )}
      </div>

      {/* import dialog (replaces 新建技能) */}
      <ImportSkillDialog open={createOpen} onOpenChange={setCreateOpen} onImported={refresh} />

      {/* SKILL.md viewer */}
      <Dialog open={viewName !== null} onOpenChange={(o) => !o && setViewName(null)}>
        <DialogContent className="flex max-h-[88vh] w-[92vw] flex-col sm:max-w-[920px]">
          <DialogHeader>
            <div className="flex items-center justify-between gap-3 pr-9">
              <div className="min-w-0">
                <DialogTitle className="font-mono">{viewName}</DialogTitle>
                <DialogDescription className="font-mono text-[11.5px]">
                  ~/.aurora/skills/{viewName}/SKILL.md
                </DialogDescription>
              </div>
              {/* lang toggle */}
              <div className="border-border bg-muted flex shrink-0 items-center gap-0.5 rounded-lg border p-0.5 select-none">
                <button
                  onClick={() => selectLang("raw")}
                  className={cn(
                    "rounded-[6px] px-2.5 py-1 text-[12px] font-medium transition-colors",
                    viewLang === "raw"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  原文
                </button>
                <button
                  onClick={() => selectLang("zh")}
                  className={cn(
                    "flex items-center gap-1 rounded-[6px] px-2.5 py-1 text-[12px] font-medium transition-colors",
                    viewLang === "zh"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <IconLanguage className="size-[13px]" />
                  中文
                </button>
              </div>
            </div>
          </DialogHeader>

          {viewLang === "raw" ? (
            <pre className="bg-muted min-h-0 flex-1 overflow-auto whitespace-pre-wrap rounded-lg p-4 text-[12.5px] leading-relaxed">
              {viewMd}
            </pre>
          ) : zhState === "ready" ? (
            <div className="flex min-h-0 flex-1 flex-col gap-2">
              <pre className="bg-muted min-h-0 flex-1 overflow-auto whitespace-pre-wrap rounded-lg p-4 text-[12.5px] leading-relaxed">
                {zhText}
              </pre>
              <div className="flex items-center justify-between select-none">
                <span className="text-muted-foreground text-[11px]">
                  机器翻译 · 已缓存，源文档变更后将自动失效
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => viewName && runTranslate(viewName)}
                >
                  <IconRefresh data-icon="inline-start" />
                  重新翻译
                </Button>
              </div>
            </div>
          ) : (
            <div className="bg-muted/40 grid min-h-0 flex-1 place-items-center rounded-lg p-6">
              <div className="flex max-w-[360px] flex-col items-center gap-3 text-center">
                {zhState === "checking" ? (
                  <p className="text-muted-foreground text-[13px] select-none">正在查找缓存…</p>
                ) : zhState === "loading" ? (
                  <p className="text-muted-foreground text-[13px] select-none">
                    正在翻译，请稍候…
                  </p>
                ) : zhState === "error" ? (
                  <>
                    <p className="text-destructive text-[13px]">{zhErr}</p>
                    <Button size="sm" onClick={() => viewName && runTranslate(viewName)}>
                      <IconLanguage data-icon="inline-start" />
                      重试翻译
                    </Button>
                  </>
                ) : (
                  // missing — no cached translation yet, ask the user
                  <>
                    <IconLanguage className="text-muted-foreground size-7" stroke={1.6} />
                    <p className="text-muted-foreground text-[13px] select-none">
                      尚未翻译该文档。是否翻译成中文？将调用「设置」中配置的模型，并缓存结果。
                    </p>
                    <Button size="sm" onClick={() => viewName && runTranslate(viewName)}>
                      <IconLanguage data-icon="inline-start" />
                      翻译成中文
                    </Button>
                  </>
                )}
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* delete confirm */}
      <AlertDialog open={pendingDelete !== null} onOpenChange={(o) => !o && setPendingDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除技能 {pendingDelete}？</AlertDialogTitle>
            <AlertDialogDescription>
              将从所有技能包中移除该技能，并删除各 agent 指向它的软链接（真实目录与外部链接不受影响），然后删除技能中心的目录。此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={onDelete}
              className={cn("bg-destructive text-white hover:bg-destructive/90")}
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
