import { useEffect, useMemo, useState } from "react";
import {
  IconSearch,
  IconPlus,
  IconPencil,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
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
import { PageHeader, Mono } from "@/components/PageHeader";
import { cn, fuzzyMatch } from "@/lib/utils";
import {
  listPacks,
  listSkills,
  createPack,
  deletePack,
  renamePack,
  addSkillToPack,
  removeSkillFromPack,
  assignPack,
  unassignPack,
  type AgentInfo,
  type Pack,
  type Skill,
} from "@/lib/api";

export function Packs({ agents }: { agents: AgentInfo[] }) {
  const [packs, setPacks] = useState<Pack[] | null>(null);
  const [allSkills, setAllSkills] = useState<Skill[]>([]);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  const [editOpen, setEditOpen] = useState(false);
  const [editOriginal, setEditOriginal] = useState<string | null>(null); // null = create
  const [formName, setFormName] = useState("");
  const [formDesc, setFormDesc] = useState("");

  const [pendingDelete, setPendingDelete] = useState<string | null>(null);

  const [addPackName, setAddPackName] = useState<string | null>(null);
  const [addQuery, setAddQuery] = useState("");

  async function refresh() {
    try {
      const [p, s] = await Promise.all([listPacks(), listSkills()]);
      setPacks(p);
      setAllSkills(s);
    } catch (e) {
      toast.error(String(e));
    }
  }
  useEffect(() => {
    refresh();
  }, []);

  const filtered = useMemo(() => {
    if (!packs) return [];
    const q = query.trim();
    return q ? packs.filter((p) => fuzzyMatch(q, p.name)) : packs;
  }, [packs, query]);

  function openCreate() {
    setEditOriginal(null);
    setFormName("");
    setFormDesc("");
    setEditOpen(true);
  }
  function openRename(p: Pack) {
    setEditOriginal(p.name);
    setFormName(p.name);
    setFormDesc(p.description);
    setEditOpen(true);
  }
  async function submitForm() {
    if (!formName.trim()) return;
    try {
      if (editOriginal === null) await createPack(formName.trim(), formDesc);
      else await renamePack(editOriginal, formName.trim(), formDesc);
      setEditOpen(false);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    }
  }

  async function togglePack(pack: Pack, agentId: string) {
    if (pack.skills.length === 0) {
      toast.error("空技能包无法分配，请先添加技能");
      return;
    }
    const key = `${pack.name}:${agentId}`;
    setBusy(key);
    const has = pack.assigned_agents.includes(agentId);
    try {
      if (has) await unassignPack(pack.name, agentId);
      else await assignPack(pack.name, agentId);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(null);
    }
  }

  const addPack = useMemo(
    () => (addPackName ? packs?.find((p) => p.name === addPackName) ?? null : null),
    [packs, addPackName],
  );
  const addAvailable = useMemo(() => {
    if (!addPack) return [];
    const inPack = new Set(addPack.skills);
    const rest = allSkills.filter((s) => !inPack.has(s.name));
    const q = addQuery.trim();
    return q ? rest.filter((s) => fuzzyMatch(q, `${s.name} ${s.description}`)) : rest;
  }, [addPack, allSkills, addQuery]);

  function openAddSkill(pack: Pack) {
    setAddQuery("");
    setAddPackName(pack.name);
  }

  async function addSkill(pack: string, skill: string) {
    try {
      await addSkillToPack(pack, skill);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    }
  }
  async function removeSkill(pack: string, skill: string) {
    try {
      await removeSkillFromPack(pack, skill);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    }
  }

  async function onDelete() {
    if (!pendingDelete) return;
    const name = pendingDelete;
    setPendingDelete(null);
    try {
      await deletePack(name);
      toast.success(`已删除技能包 ${name}`);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    }
  }

  return (
    <>
      <PageHeader title="技能包">
        把常用技能打包，一次性分配给 agent · 定义存于 <Mono>~/.aurora/packs.json</Mono>
      </PageHeader>

      <div className="flex flex-wrap items-center gap-2.5 px-6 py-3.5">
        <div className="relative min-w-[200px] flex-1">
          <IconSearch className="text-muted-foreground absolute left-3 top-1/2 size-[15px] -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索技能包…"
            className="pl-9"
          />
        </div>
        <Button onClick={openCreate}>
          <IconPlus data-icon="inline-start" />
          新建技能包
        </Button>
      </div>

      <div className="text-muted-foreground px-6 pb-1 text-[11.5px]">
        点击包底部的 agent 徽标，将包内全部技能一次性软链接到该 agent
      </div>

      <div className="flex-1 overflow-auto px-4 pb-6 pt-2">
        {packs === null ? (
          <div className="flex flex-col gap-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-[150px] rounded-[10px]" />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>没有技能包</EmptyTitle>
              <EmptyDescription>
                {query ? "没有匹配的技能包。" : "点击「新建技能包」创建第一个。"}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className="flex flex-col gap-3">
            {filtered.map((pack) => {
              const available = allSkills.filter((s) => !pack.skills.includes(s.name));
              return (
                <div key={pack.name} className="border-border hover:border-primary/30 rounded-[10px] border p-4 transition-colors">
                  <div className="flex items-center gap-2.5">
                    <span className="font-heading text-sm font-semibold">{pack.name}</span>
                    <span className="text-muted-foreground border-border rounded-full border px-2 py-0.5 text-[11px]">
                      {pack.skills.length} 个技能
                    </span>
                    <div className="ml-auto flex gap-1.5">
                      <Button variant="outline" size="icon" className="size-[30px]" title="重命名" onClick={() => openRename(pack)}>
                        <IconPencil className="size-[15px]" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon"
                        className="hover:border-destructive hover:text-destructive size-[30px]"
                        title="删除技能包"
                        onClick={() => setPendingDelete(pack.name)}
                      >
                        <IconTrash className="size-[15px]" />
                      </Button>
                    </div>
                  </div>

                  {pack.description && (
                    <p className="text-muted-foreground mt-1 text-[12.5px]">{pack.description}</p>
                  )}

                  <div className="mt-3 flex flex-wrap items-center gap-1.5">
                    {pack.skills.map((s) => (
                      <span
                        key={s}
                        className="bg-muted border-border inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-[11px] font-medium"
                      >
                        <span className="font-mono">{s}</span>
                        <button
                          onClick={() => removeSkill(pack.name, s)}
                          title="从包中移出"
                          className="text-muted-foreground hover:text-destructive"
                        >
                          <IconX className="size-3" />
                        </button>
                      </span>
                    ))}

                    <button
                      onClick={() => openAddSkill(pack)}
                      disabled={available.length === 0}
                      className="text-primary border-primary/40 hover:bg-primary/5 inline-flex items-center gap-1 rounded-md border border-dashed px-2.5 py-1 text-[11px] font-medium transition-colors disabled:opacity-40"
                    >
                      <IconPlus className="size-3" /> 添加技能
                    </button>
                  </div>

                  <div className="border-border mt-3 flex flex-wrap items-center gap-2 border-t border-dashed pt-3">
                    <span className="text-muted-foreground text-[11px]">分配给：</span>
                    {agents.map((a) => (
                      <AgentBadge
                        key={a.id}
                        agent={a}
                        active={pack.assigned_agents.includes(a.id)}
                        busy={busy === `${pack.name}:${a.id}`}
                        onToggle={() => togglePack(pack, a.id)}
                      />
                    ))}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* add-skill picker dialog */}
      <Dialog open={addPackName !== null} onOpenChange={(o) => !o && setAddPackName(null)}>
        <DialogContent className="gap-0 p-0 sm:max-w-[440px]">
          <DialogHeader className="px-5 pb-3 pt-5">
            <DialogTitle>添加技能到 {addPackName}</DialogTitle>
            <DialogDescription>从技能中心选择技能，可连续添加多个。</DialogDescription>
          </DialogHeader>
          <div className="px-5 pb-3">
            <div className="relative">
              <IconSearch className="text-muted-foreground absolute left-3 top-1/2 size-[15px] -translate-y-1/2" />
              <Input
                autoFocus
                value={addQuery}
                onChange={(e) => setAddQuery(e.target.value)}
                placeholder="搜索技能名称或描述…"
                className="pl-9"
              />
            </div>
          </div>
          <ScrollArea className="border-border max-h-[340px] border-t">
            <div className="flex flex-col p-1.5">
              {addAvailable.length === 0 ? (
                <div className="text-muted-foreground px-3 py-10 text-center text-[12.5px]">
                  {addQuery ? "没有匹配的技能。" : "技能中心的技能都已在包内。"}
                </div>
              ) : (
                addAvailable.map((s) => (
                  <button
                    key={s.name}
                    onClick={() => addSkill(addPackName!, s.name)}
                    className="hover:bg-muted flex items-start gap-2 rounded-md px-2.5 py-2 text-left transition-colors"
                  >
                    <div className="min-w-0 flex-1">
                      <span className="font-mono text-[12.5px] font-semibold">{s.name}</span>
                      <p className="text-muted-foreground line-clamp-1 text-[11.5px]">
                        {s.description || "—"}
                      </p>
                    </div>
                  </button>
                ))
              )}
            </div>
          </ScrollArea>
          <DialogFooter className="px-5 py-3.5">
            <Button variant="outline" onClick={() => setAddPackName(null)}>
              完成
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* create / rename dialog */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editOriginal === null ? "新建技能包" : "重命名技能包"}</DialogTitle>
            <DialogDescription>技能包定义保存在 packs.json。</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <Input
              value={formName}
              onChange={(e) => setFormName(e.target.value)}
              placeholder="技能包名称"
            />
            <Input
              value={formDesc}
              onChange={(e) => setFormDesc(e.target.value)}
              placeholder="描述（可选）"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>
              取消
            </Button>
            <Button onClick={submitForm} disabled={!formName.trim()}>
              {editOriginal === null ? "创建" : "保存"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* delete confirm */}
      <AlertDialog open={pendingDelete !== null} onOpenChange={(o) => !o && setPendingDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除技能包 {pendingDelete}？</AlertDialogTitle>
            <AlertDialogDescription>
              仅删除技能包定义，不影响技能中心里的技能，也不会移除已分配到 agent 的软链接。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={onDelete} className={cn("bg-destructive text-white hover:bg-destructive/90")}>
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
