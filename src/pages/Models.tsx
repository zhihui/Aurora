import { useEffect, useMemo, useState } from "react";
import {
  IconSearch,
  IconPlus,
  IconPencil,
  IconTrash,
  IconExternalLink,
} from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
import { PageHeader, Mono } from "@/components/PageHeader";
import { cn, fuzzyMatch } from "@/lib/utils";
import {
  listProviders,
  createProvider,
  updateProvider,
  deleteProvider,
  addModel,
  updateModel,
  removeModel,
  type Provider,
  type ModelInfo,
} from "@/lib/api";

type ProviderForm = {
  key: string;
  name: string;
  endpointOpenai: string;
  endpointClaude: string;
  site: string;
  apiKey: string;
};

const EMPTY_FORM: ProviderForm = {
  key: "",
  name: "",
  endpointOpenai: "",
  endpointClaude: "",
  site: "",
  apiKey: "",
};

/// Model modality options. "text" is always selected and cannot be toggled off;
/// "image" and "video" are optional.
const CAPABILITY_OPTIONS: { id: string; label: string }[] = [
  { id: "text", label: "文本" },
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
];

/// Always-on "text" baseline; image/video are user-selectable.
const DEFAULT_CAPS = ["text"];

/// Parse an optional positive integer from a string input. Empty / invalid / ≤0
/// values resolve to null (i.e. "unset").
function parseOptInt(s: string): number | null {
  const t = s.trim();
  if (!t) return null;
  const n = Number(t);
  if (!Number.isFinite(n) || n <= 0) return null;
  return Math.floor(n);
}

/// Format a token count for display, e.g. 128000 -> "128,000".
function fmtTokens(v: number | null): string | null {
  if (v == null) return null;
  return v.toLocaleString("en-US");
}

/// Human label for a capability id.
function capLabel(id: string): string {
  return CAPABILITY_OPTIONS.find((o) => o.id === id)?.label ?? id;
}

export function Models() {
  const [providers, setProviders] = useState<Provider[] | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);

  // provider create/edit dialog
  const [editOpen, setEditOpen] = useState(false);
  const [editOriginalKey, setEditOriginalKey] = useState<string | null>(null); // null = create
  const [form, setForm] = useState<ProviderForm>(EMPTY_FORM);

  // add-model dialog
  const [modelOpenFor, setModelOpenFor] = useState<string | null>(null);
  const [modelId, setModelId] = useState("");
  const [modelName, setModelName] = useState("");
  const [modelCtx, setModelCtx] = useState("");
  const [modelMax, setModelMax] = useState("");
  const [modelCaps, setModelCaps] = useState<string[]>(DEFAULT_CAPS);

  // edit-model dialog
  const [editModelFor, setEditModelFor] = useState<string | null>(null);
  const [editModelId, setEditModelId] = useState("");
  const [editModelName, setEditModelName] = useState("");
  const [editModelCtx, setEditModelCtx] = useState("");
  const [editModelMax, setEditModelMax] = useState("");
  const [editModelCaps, setEditModelCaps] = useState<string[]>(DEFAULT_CAPS);

  // delete confirm
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);
  const [pendingDeleteModel, setPendingDeleteModel] = useState<{ provider: string; id: string } | null>(null);

  async function refresh(preserveSelection?: boolean) {
    try {
      const list = await listProviders();
      setProviders(list);
      // Keep a valid selection; otherwise fall back to the first provider.
      if (preserveSelection && selectedKey && list.some((p) => p.key === selectedKey)) {
        return;
      }
      setSelectedKey((prev) => {
        if (prev && list.some((p) => p.key === prev)) return prev;
        return list[0]?.key ?? null;
      });
    } catch (e) {
      toast.error(String(e));
      setProviders([]);
    }
  }
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const filtered = useMemo(() => {
    if (!providers) return [];
    const q = query.trim();
    return q
      ? providers.filter((p) =>
          fuzzyMatch(q, `${p.name} ${p.key} ${p.endpoint_openai} ${p.endpoint_claude}`),
        )
      : providers;
  }, [providers, query]);

  const selected = useMemo(
    () => providers?.find((p) => p.key === selectedKey) ?? null,
    [providers, selectedKey],
  );

  function openCreate() {
    setEditOriginalKey(null);
    setForm(EMPTY_FORM);
    setEditOpen(true);
  }
  function openEdit(p: Provider) {
    setEditOriginalKey(p.key);
    setForm({
      key: p.key,
      name: p.name,
      endpointOpenai: p.endpoint_openai,
      endpointClaude: p.endpoint_claude,
      site: p.site,
      apiKey: "",
    });
    setEditOpen(true);
  }

  async function submitProvider() {
    if (!form.name.trim() || !form.key.trim()) {
      toast.error("名称与英文标识不能为空");
      return;
    }
    setBusy(true);
    try {
      // Empty key field → keep stored key (send null) when editing.
      const apiKey = form.apiKey.trim() === "" ? null : form.apiKey;
      let saved: Provider;
      if (editOriginalKey === null) {
        saved = await createProvider(
          form.key,
          form.name,
          form.endpointOpenai,
          form.endpointClaude,
          form.site,
          apiKey,
        );
      } else {
        saved = await updateProvider(
          editOriginalKey,
          form.key,
          form.name,
          form.endpointOpenai,
          form.endpointClaude,
          form.site,
          apiKey,
        );
      }
      setEditOpen(false);
      await refresh(true);
      setSelectedKey(saved.key);
      toast.success(editOriginalKey === null ? "已创建模型供应商" : "已保存配置");
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onAddModel() {
    if (!modelOpenFor || !modelId.trim()) return;
    setBusy(true);
    try {
      const saved = await addModel(
        modelOpenFor,
        modelId,
        modelName,
        parseOptInt(modelCtx),
        parseOptInt(modelMax),
        modelCaps,
      );
      setProviders((prev) =>
        prev ? prev.map((p) => (p.key === saved.key ? saved : p)) : prev,
      );
      setModelId("");
      setModelName("");
      setModelCtx("");
      setModelMax("");
      setModelCaps(DEFAULT_CAPS);
      setModelOpenFor(null);
      toast.success("已添加模型");
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onUpdateModel() {
    if (!editModelFor || !editModelId) return;
    setBusy(true);
    try {
      const saved = await updateModel(
        editModelFor,
        editModelId,
        editModelName,
        parseOptInt(editModelCtx),
        parseOptInt(editModelMax),
        editModelCaps,
      );
      setProviders((prev) =>
        prev ? prev.map((p) => (p.key === saved.key ? saved : p)) : prev,
      );
      setEditModelFor(null);
      toast.success("已保存模型");
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onRemoveModel() {
    if (!pendingDeleteModel) return;
    const { provider, id } = pendingDeleteModel;
    setPendingDeleteModel(null);
    setBusy(true);
    try {
      const saved = await removeModel(provider, id);
      setProviders((prev) =>
        prev ? prev.map((p) => (p.key === saved.key ? saved : p)) : prev,
      );
      toast.success(`已移除模型 ${id}`);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onDeleteProvider() {
    if (!pendingDelete) return;
    const key = pendingDelete;
    setPendingDelete(null);
    setBusy(true);
    try {
      await deleteProvider(key);
      if (selectedKey === key) setSelectedKey(null);
      await refresh();
      toast.success("已移除模型供应商");
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  function hostOf(url: string) {
    try {
      return new URL(url).host;
    } catch {
      return "";
    }
  }

  return (
    <>
      <PageHeader title="模型中心">
        集中管理各模型供应商及其模型 · 定义存于 <Mono>~/.aurora/config.json</Mono> · 密钥只保存不显示
      </PageHeader>

      <div className="grid min-h-0 flex-1 grid-cols-[260px_1fr] overflow-hidden max-[1080px]:grid-cols-[220px_1fr] max-[820px]:grid-cols-1 max-[820px]:grid-rows-[minmax(0,200px)_1fr]">
        {/* ─── left: provider list ─── */}
        <div className="border-border flex min-h-0 flex-col border-r max-[820px]:border-b max-[820px]:border-r-0">
          <div className="flex items-center gap-2 p-2">
            <div className="relative min-w-0 flex-1">
              <IconSearch className="text-muted-foreground absolute left-2.5 top-1/2 size-[15px] -translate-y-1/2" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索模型供应商…"
                className="h-[30px] pl-8"
              />
            </div>
            <Button
              variant="outline"
              size="icon"
              className="size-[30px] shrink-0"
              title="新建模型供应商"
              onClick={openCreate}
            >
              <IconPlus className="size-[15px]" />
            </Button>
          </div>

          <div className="min-h-0 flex-1 overflow-auto p-1.5">
            {providers === null ? (
              <div className="flex flex-col gap-1.5 p-1">
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className="h-[52px] rounded-lg" />
                ))}
              </div>
            ) : filtered.length === 0 ? (
              <div className="text-muted-foreground px-2 py-6 text-center text-[12px]">
                {query ? "没有匹配的模型供应商。" : "还没有模型供应商，点击 + 新建。"}
              </div>
            ) : (
              filtered.map((p) => (
                <button
                  key={p.key}
                  onClick={() => setSelectedKey(p.key)}
                  className={cn(
                    "flex w-full items-center gap-2.5 rounded-lg border border-transparent p-2 text-left transition-colors",
                    selectedKey === p.key
                      ? "bg-primary/10 border-primary/20"
                      : "hover:bg-foreground/5",
                  )}
                >
                  <div
                    className="grid size-[30px] shrink-0 place-items-center rounded-lg text-[12px] font-bold text-white"
                    style={{ background: p.color }}
                  >
                    {p.initials}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5 text-[12.5px] font-semibold">
                      <span className="truncate">{p.name}</span>
                      <span className="text-muted-foreground text-[10.5px] font-medium">
                        {p.models.length}
                      </span>
                    </div>
                    <div className="text-muted-foreground truncate font-mono text-[11px]">
                      {hostOf(p.endpoint_openai) ||
                        hostOf(p.endpoint_claude) ||
                        p.endpoint_openai ||
                        p.endpoint_claude ||
                        "未配置端点"}
                    </div>
                  </div>
                  <span
                    title={p.has_key ? "密钥已保存" : "尚未保存密钥"}
                    className={cn(
                      "size-[7px] shrink-0 rounded-full border",
                      p.has_key
                        ? "border-emerald-500 bg-emerald-500"
                        : "border-border bg-transparent",
                    )}
                  />
                </button>
              ))
            )}
          </div>
        </div>

        {/* ─── right: detail ─── */}
        <div className="min-h-0 overflow-auto px-4 pb-10 pt-4 sm:px-6 max-[820px]:pt-3">
          {!selected ? (
            <Empty className="border-border mt-6">
              <EmptyHeader>
                <EmptyTitle>未选择模型供应商</EmptyTitle>
                <EmptyDescription>
                  从左侧选择一个模型供应商，或点击 + 新建。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <>
              {/* header */}
              <div className="flex flex-wrap items-center gap-3">
                <div
                  className="grid size-[42px] shrink-0 place-items-center rounded-[11px] text-[17px] font-bold text-white"
                  style={{ background: selected.color }}
                >
                  {selected.initials}
                </div>
                <div className="min-w-0">
                  <div className="font-heading flex items-center gap-2 text-[19px] font-bold tracking-tight">
                    <span className="truncate">{selected.name}</span>
                    <span className="text-primary border-primary/20 bg-primary/10 rounded-md border px-2 py-0.5 font-mono text-[11px] font-semibold">
                      {selected.key}
                    </span>
                  </div>
                  {selected.site && (
                    <a
                      href={selected.site}
                      target="_blank"
                      rel="noreferrer"
                      className="text-muted-foreground hover:text-primary mt-0.5 inline-flex items-center gap-1 text-[11.5px]"
                    >
                      <IconExternalLink className="size-3" />
                      {hostOf(selected.site) || selected.site}
                    </a>
                  )}
                </div>
                <div className="ml-auto flex gap-1.5">
                  <Button variant="outline" size="icon" className="size-[30px]" title="编辑" onClick={() => openEdit(selected)}>
                    <IconPencil className="size-[15px]" />
                  </Button>
                  <Button
                    variant="outline"
                    size="icon"
                    className="hover:border-destructive hover:text-destructive size-[30px]"
                    title="移除模型供应商"
                    onClick={() => setPendingDelete(selected.key)}
                  >
                    <IconTrash className="size-[15px]" />
                  </Button>
                </div>
              </div>

              {/* read-only connection summary */}
              <div className="mt-5 grid grid-cols-1 gap-2.5 lg:grid-cols-2">
                <SummaryField
                  label="OpenAI 兼容端点"
                  value={selected.endpoint_openai}
                  mono
                  placeholder="未配置"
                />
                <SummaryField
                  label="Claude 兼容端点"
                  value={selected.endpoint_claude}
                  mono
                  placeholder="未配置"
                />
                <SummaryField label="英文标识" value={selected.key} mono />
                <div className="flex flex-col gap-1">
                  <span className="text-muted-foreground text-[11.5px] font-medium">API 密钥</span>
                  <div className="flex items-center gap-2">
                    <span
                      className={cn(
                        "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-semibold",
                        selected.has_key
                          ? "bg-emerald-500/10 text-emerald-600"
                          : "border-border bg-muted text-muted-foreground border",
                      )}
                    >
                      <span
                        className={cn(
                          "size-1.5 rounded-full",
                          selected.has_key ? "bg-emerald-500" : "bg-muted-foreground/40",
                        )}
                      />
                      {selected.has_key ? "密钥已保存" : "尚未保存密钥"}
                    </span>
                  </div>
                </div>
                <SummaryField label="官网 URL" value={selected.site} mono placeholder="—" />
              </div>

              <div className="mt-2 flex justify-end">
                <Button variant="outline" size="sm" onClick={() => openEdit(selected)}>
                  <IconPencil data-icon="inline-start" />
                  编辑配置
                </Button>
              </div>

              {/* models */}
              <div className="text-muted-foreground mt-6 px-0.5 pb-2 text-[10.5px] font-semibold uppercase tracking-wider">
                模型
                <span className="text-muted-foreground ml-2 font-medium normal-case tracking-normal">
                  · {selected.models.length} 个
                </span>
              </div>

              <div className="flex flex-col gap-1.5">
                {selected.models.length === 0 ? (
                  <div className="text-muted-foreground border-border rounded-lg border border-dashed py-6 text-center text-[12.5px]">
                    尚未添加模型
                  </div>
                ) : (
                  selected.models.map((m: ModelInfo) => (
                    <div
                      key={m.id}
                      className="bg-muted/60 border-border flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1.5 rounded-lg border px-3 py-2"
                    >
                      <span className="min-w-0 break-all font-mono text-[12.5px] font-semibold">
                        {m.id}
                      </span>
                      {m.name && (
                        <span className="text-muted-foreground min-w-0 truncate text-[12px]">
                          {m.name}
                        </span>
                      )}
                      {(m.context_window != null || m.max_output != null) && (
                        <span className="flex flex-wrap items-center gap-1.5">
                          {m.context_window != null && (
                            <span className="bg-background/70 text-muted-foreground rounded-md px-1.5 py-0.5 text-[10.5px] font-medium tabular-nums">
                              上下文 {fmtTokens(m.context_window)}
                            </span>
                          )}
                          {m.max_output != null && (
                            <span className="bg-background/70 text-muted-foreground rounded-md px-1.5 py-0.5 text-[10.5px] font-medium tabular-nums">
                              最大输出 {fmtTokens(m.max_output)}
                            </span>
                          )}
                        </span>
                      )}
                      {m.capabilities
                        ?.filter((c) => c !== "text")
                        .map((c) => (
                          <span
                            key={c}
                            className="bg-primary/10 text-primary rounded-md px-1.5 py-0.5 text-[10.5px] font-medium"
                          >
                            {capLabel(c)}
                          </span>
                        ))}
                      <span className="ml-auto flex items-center gap-0.5">
                        <button
                          onClick={() => {
                            setEditModelFor(selected.key);
                            setEditModelId(m.id);
                            setEditModelName(m.name);
                            setEditModelCtx(m.context_window != null ? String(m.context_window) : "");
                            setEditModelMax(m.max_output != null ? String(m.max_output) : "");
                            setEditModelCaps(
                              m.capabilities?.length ? m.capabilities : DEFAULT_CAPS,
                            );
                          }}
                          title="编辑模型"
                          className="text-muted-foreground hover:bg-foreground/10 hover:text-foreground grid size-[26px] place-items-center rounded-md transition-colors"
                        >
                          <IconPencil className="size-[13px]" />
                        </button>
                        <button
                          onClick={() => setPendingDeleteModel({ provider: selected.key, id: m.id })}
                          title="移除模型"
                          className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive grid size-[26px] place-items-center rounded-md transition-colors"
                        >
                          <IconTrash className="size-[14px]" />
                        </button>
                      </span>
                    </div>
                  ))
                )}

                <button
                  onClick={() => {
                    setModelId("");
                    setModelName("");
                    setModelCtx("");
                    setModelMax("");
                    setModelCaps(DEFAULT_CAPS);
                    setModelOpenFor(selected.key);
                  }}
                  className="text-primary border-primary/30 hover:bg-primary/5 mt-1 inline-flex w-fit items-center gap-1 rounded-lg border border-dashed px-3 py-1.5 text-[11.5px] font-medium transition-colors"
                >
                  <IconPlus className="size-[13px]" /> 添加模型
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      {/* ─── provider create / edit dialog ─── */}
      <Dialog open={editOpen} onOpenChange={(o) => !busy && setEditOpen(o)}>
        <DialogContent className="sm:max-w-[560px]">
          <DialogHeader>
            <DialogTitle>{editOriginalKey === null ? "新建模型供应商" : "编辑模型供应商"}</DialogTitle>
            <DialogDescription>密钥只保存不显示，编辑时留空表示保持原密钥不变。</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium">名称</span>
                <Input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="OpenAI"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium">英文标识</span>
                <Input
                  value={form.key}
                  onChange={(e) => setForm({ ...form, key: e.target.value.toLowerCase() })}
                  placeholder="openai"
                  className="font-mono text-[12.5px]"
                  spellCheck={false}
                />
                <span className="text-muted-foreground text-[11px]">
                  唯一标识，仅限小写字母 / 数字 / - _
                </span>
              </label>
            </div>
            <div className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">
                API 端点
              </span>
              <span className="text-muted-foreground text-[11px]">
                可同时填写，也可只填一个 · 按需配置该模型供应商支持的 API 风格
              </span>
            </div>
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">
                OpenAI 兼容端点
              </span>
              <Input
                value={form.endpointOpenai}
                onChange={(e) => setForm({ ...form, endpointOpenai: e.target.value })}
                placeholder="https://api.openai.com/v1"
                className="font-mono text-[12.5px]"
                spellCheck={false}
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">
                Claude 兼容端点
              </span>
              <Input
                value={form.endpointClaude}
                onChange={(e) => setForm({ ...form, endpointClaude: e.target.value })}
                placeholder="https://api.anthropic.com"
                className="font-mono text-[12.5px]"
                spellCheck={false}
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">官网 URL（可选）</span>
              <Input
                value={form.site}
                onChange={(e) => setForm({ ...form, site: e.target.value })}
                placeholder="https://openai.com"
                className="font-mono text-[12.5px]"
                spellCheck={false}
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">
                API 密钥
                <span className="text-muted-foreground/70 ml-1.5 font-normal">仅可保存，不可查看</span>
              </span>
              <Input
                type="password"
                value={form.apiKey}
                onChange={(e) => setForm({ ...form, apiKey: e.target.value })}
                placeholder={editOriginalKey ? "已保存密钥 · 留空则保持不变" : "sk-…"}
                className="font-mono text-[12.5px]"
                autoComplete="off"
                spellCheck={false}
              />
            </label>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)} disabled={busy}>
              取消
            </Button>
            <Button onClick={submitProvider} disabled={busy || !form.name.trim() || !form.key.trim()}>
              {busy ? "保存中…" : editOriginalKey === null ? "创建" : "保存"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ─── add-model dialog ─── */}
      <Dialog open={modelOpenFor !== null} onOpenChange={(o) => !busy && !o && setModelOpenFor(null)}>
        <DialogContent className="sm:max-w-[440px]">
          <DialogHeader>
            <DialogTitle>添加模型到 {selected?.name ?? ""}</DialogTitle>
            <DialogDescription>模型 ID 将在调用时使用。</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">模型 ID</span>
              <Input
                autoFocus
                value={modelId}
                onChange={(e) => setModelId(e.target.value)}
                placeholder="gpt-4o-mini"
                className="font-mono text-[12.5px]"
                spellCheck={false}
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">显示名称（可选）</span>
              <Input
                value={modelName}
                onChange={(e) => setModelName(e.target.value)}
                placeholder="GPT-4o mini"
              />
            </label>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium">
                  上下文窗口大小（可选）
                </span>
                <Input
                  value={modelCtx}
                  onChange={(e) => setModelCtx(e.target.value)}
                  placeholder="128000"
                  inputMode="numeric"
                  className="tabular-nums"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium">
                  最大输出长度（可选）
                </span>
                <Input
                  value={modelMax}
                  onChange={(e) => setModelMax(e.target.value)}
                  placeholder="16384"
                  inputMode="numeric"
                  className="tabular-nums"
                />
              </label>
            </div>
            <span className="text-muted-foreground text-[11px]">
              上下文与最大输出均为整数 token 数，可留空。
            </span>
            <div className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">模型能力</span>
              <CapabilityPicker value={modelCaps} onChange={setModelCaps} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setModelOpenFor(null)} disabled={busy}>
              取消
            </Button>
            <Button onClick={onAddModel} disabled={busy || !modelId.trim()}>
              {busy ? "添加中…" : "添加"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ─── edit-model dialog ─── */}
      <Dialog open={editModelFor !== null} onOpenChange={(o) => !busy && !o && setEditModelFor(null)}>
        <DialogContent className="sm:max-w-[440px]">
          <DialogHeader>
            <DialogTitle>编辑模型</DialogTitle>
            <DialogDescription>模型 ID 不可修改，仅用于调用标识。</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">模型 ID</span>
              <Input value={editModelId} disabled className="font-mono text-[12.5px]" />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">显示名称（可选）</span>
              <Input
                value={editModelName}
                onChange={(e) => setEditModelName(e.target.value)}
                placeholder="GPT-4o mini"
              />
            </label>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium">
                  上下文窗口大小（可选）
                </span>
                <Input
                  value={editModelCtx}
                  onChange={(e) => setEditModelCtx(e.target.value)}
                  placeholder="128000"
                  inputMode="numeric"
                  className="tabular-nums"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium">
                  最大输出长度（可选）
                </span>
                <Input
                  value={editModelMax}
                  onChange={(e) => setEditModelMax(e.target.value)}
                  placeholder="16384"
                  inputMode="numeric"
                  className="tabular-nums"
                />
              </label>
            </div>
            <span className="text-muted-foreground text-[11px]">
              上下文与最大输出均为整数 token 数，留空表示不设置。
            </span>
            <div className="flex flex-col gap-1">
              <span className="text-muted-foreground text-[11.5px] font-medium">模型能力</span>
              <CapabilityPicker value={editModelCaps} onChange={setEditModelCaps} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditModelFor(null)} disabled={busy}>
              取消
            </Button>
            <Button onClick={onUpdateModel} disabled={busy}>
              {busy ? "保存中…" : "保存"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ─── delete provider confirm ─── */}
      <AlertDialog open={pendingDelete !== null} onOpenChange={(o) => !o && setPendingDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>移除模型供应商 {pendingDelete}？</AlertDialogTitle>
            <AlertDialogDescription>
              将删除该模型供应商及其全部模型配置，操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={onDeleteProvider}
              className={cn("bg-destructive text-white hover:bg-destructive/90")}
            >
              移除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* ─── delete model confirm ─── */}
      <AlertDialog
        open={pendingDeleteModel !== null}
        onOpenChange={(o) => !o && setPendingDeleteModel(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>移除模型 {pendingDeleteModel?.id}？</AlertDialogTitle>
            <AlertDialogDescription>从该模型供应商中移除此模型，操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={onRemoveModel}
              className={cn("bg-destructive text-white hover:bg-destructive/90")}
            >
              移除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

/// Multi-select chips for model modalities. "text" is always on and disabled;
/// "image"/"video" toggle on click.
function CapabilityPicker({
  value,
  onChange,
}: {
  value: string[];
  onChange: (v: string[]) => void;
}) {
  function toggle(id: string) {
    if (value.includes(id)) {
      onChange(value.filter((c) => c !== id));
    } else {
      onChange([...value, id]);
    }
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {CAPABILITY_OPTIONS.map((o) => {
        const on = value.includes(o.id);
        const locked = o.id === "text";
        return (
          <button
            key={o.id}
            type="button"
            disabled={locked}
            onClick={() => toggle(o.id)}
            className={cn(
              "rounded-md border px-2.5 py-1 text-[12px] font-medium transition-colors",
              on
                ? "border-primary/30 bg-primary/10 text-primary"
                : "border-border bg-transparent text-muted-foreground hover:bg-foreground/5",
              locked && "cursor-not-allowed opacity-100",
            )}
            title={locked ? "文本为默认能力，不可取消" : undefined}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

function SummaryField({
  label,
  value,
  mono,
  placeholder,
}: {
  label: string;
  value: string;
  mono?: boolean;
  placeholder?: string;
}) {
  return (
    <div className="border-border flex min-w-0 flex-col gap-1 rounded-lg border p-3">
      <span className="text-muted-foreground text-[11px] font-semibold uppercase tracking-wider">
        {label}
      </span>
      <span
        className={cn(
          "min-w-0 break-all text-[12.5px]",
          mono && "font-mono",
          !value && "text-muted-foreground",
        )}
        title={value || undefined}
      >
        {value || placeholder || "—"}
      </span>
    </div>
  );
}
