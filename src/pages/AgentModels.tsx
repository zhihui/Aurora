import { useEffect, useMemo, useState } from "react";
import {
  IconCheck,
  IconChevronDown,
  IconCpu2,
  IconHelp,
  IconRefresh,
  IconSearch,
} from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { PageHeader, Mono } from "@/components/PageHeader";
import { cn, fuzzyMatch } from "@/lib/utils";
import {
  listAgentModelConfigs,
  listProviders,
  previewAgentModelConfig,
  setAgentModelConfig,
  syncAllAgentModelConfigs,
  syncAgentModelConfig,
  type AgentModelConfig,
  type AgentModelFile,
  type AgentModelOptions,
  type ModelInfo,
  type Provider,
} from "@/lib/api";

const EMPTY_OPTIONS: AgentModelOptions = {
  claude: {
    same_model_for_slots: true,
    haiku_model: "",
    sonnet_model: "",
    opus_model: "",
  },
  kimi: {
    provider_type: "openai",
    capabilities: ["image_in", "video_in", "thinking"],
  },
  opencode: {
    provider_type: "openai",
  },
  codex: {
    env_key: "",
  },
};

function cloneOptions(options?: AgentModelOptions): AgentModelOptions {
  return {
    claude: { ...EMPTY_OPTIONS.claude, ...(options?.claude ?? {}) },
    kimi: {
      ...EMPTY_OPTIONS.kimi,
      ...(options?.kimi ?? {}),
      capabilities: [...(options?.kimi?.capabilities ?? EMPTY_OPTIONS.kimi.capabilities)],
    },
    opencode: { ...EMPTY_OPTIONS.opencode, ...(options?.opencode ?? {}) },
    codex: { ...EMPTY_OPTIONS.codex, ...(options?.codex ?? {}) },
  };
}

function formatTokens(v: number | null) {
  if (v == null) return "—";
  if (v >= 1_000_000) return `${Math.round(v / 1000) / 1000}M`;
  if (v >= 1000) return `${Math.round(v / 1000)}K`;
  return String(v);
}

function endpointFor(agent: AgentModelConfig, provider: Provider) {
  return agent.endpoint_kind === "claude" ? provider.endpoint_claude : provider.endpoint_openai;
}

/// Agents whose compatibility type the user can pick (openai or anthropic);
/// for these a provider is usable with either endpoint present.
function hasSelectableType(agentId: string) {
  return agentId === "kimi" || agentId === "opencode";
}

/// For agents with a selectable type, a provider is usable as long as it has
/// at least one of the two endpoints. All others require the single endpoint
/// of their fixed kind.
function providerCompatible(agent: AgentModelConfig, provider: Provider) {
  if (hasSelectableType(agent.agent.id)) {
    return Boolean(provider.endpoint_openai.trim()) || Boolean(provider.endpoint_claude.trim());
  }
  return Boolean(endpointFor(agent, provider).trim());
}

/// Compatibility types a provider offers, in display order. openai first (the
/// common case), then anthropic — but only the ones actually configured.
function availableTypes(provider: Provider | null): ("openai" | "anthropic")[] {
  if (!provider) return [];
  const types: ("openai" | "anthropic")[] = [];
  if (provider.endpoint_openai.trim()) types.push("openai");
  if (provider.endpoint_claude.trim()) types.push("anthropic");
  return types;
}

/// Display name for a compatibility type. The stored value stays the raw API
/// string ("openai"/"anthropic"); only the UI shows the capitalized form.
function typeLabel(type: string): string {
  return type === "anthropic" ? "Anthropic" : type === "openai" ? "OpenAI" : type || "—";
}

/// Human label for the endpoint(s) a compatible provider exposes for this agent.
function providerEndpointLabel(agent: AgentModelConfig, provider: Provider) {
  if (hasSelectableType(agent.agent.id)) {
    const types = availableTypes(provider);
    return types
      .map((t) => (t === "anthropic" ? "Claude" : "OpenAI") + " 端点")
      .join(" / ");
  }
  return agent.endpoint_kind === "claude" ? "Claude 端点" : "OpenAI 端点";
}

function modelLabel(model: ModelInfo | null) {
  if (!model) return "未选择模型";
  return model.name ? `${model.id} · ${model.name}` : model.id;
}

function defaultEnvKey(provider: string) {
  const key = provider.replace(/[^A-Za-z0-9]/g, "_").toUpperCase();
  return `${key}_MODEL_API_KEY`;
}

export function AgentModels() {
  const [configs, setConfigs] = useState<AgentModelConfig[] | null>(null);
  const [providers, setProviders] = useState<Provider[] | null>(null);
  const [agentId, setAgentId] = useState<string | null>(null);
  const [providerKey, setProviderKey] = useState("");
  const [modelId, setModelId] = useState("");
  const [options, setOptions] = useState<AgentModelOptions>(EMPTY_OPTIONS);
  const [preview, setPreview] = useState<AgentModelFile[]>([]);
  const [query, setQuery] = useState("");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  async function refresh(keepAgent?: string) {
    try {
      const [cfgs, ps] = await Promise.all([listAgentModelConfigs(), listProviders()]);
      setConfigs(cfgs);
      setProviders(ps);
      const nextId = keepAgent ?? agentId ?? cfgs[0]?.agent.id ?? null;
      setAgentId(nextId);
      const cfg = cfgs.find((c) => c.agent.id === nextId) ?? cfgs[0];
      if (cfg) {
        setProviderKey(cfg.provider);
        setModelId(cfg.model);
        setOptions(cloneOptions(cfg.options));
      }
    } catch (e) {
      toast.error(String(e));
      setConfigs([]);
      setProviders([]);
    }
  }

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const current = configs?.find((c) => c.agent.id === agentId) ?? null;
  const selectedProvider = providers?.find((p) => p.key === providerKey) ?? null;
  const selectedModel = selectedProvider?.models.find((m) => m.id === modelId) ?? null;

  useEffect(() => {
    if (!current) return;
    const cfg = configs?.find((c) => c.agent.id === current.agent.id);
    if (!cfg) return;
    setProviderKey(cfg.provider);
    setModelId(cfg.model);
    setOptions(cloneOptions(cfg.options));
    setPreview([]);
    setPickerOpen(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId]);

  useEffect(() => {
    let cancelled = false;
    async function loadPreview() {
      if (!current || !providerKey || !modelId) {
        setPreview([]);
        return;
      }
      try {
        const files = await previewAgentModelConfig(current.agent.id, providerKey, modelId, options);
        if (!cancelled) setPreview(files);
      } catch {
        if (!cancelled) setPreview([]);
      }
    }
    loadPreview();
    return () => {
      cancelled = true;
    };
  }, [current, providerKey, modelId, options]);

  // For agents with a selectable compatibility type (Kimi / Opencode), the
  // type must be one the selected provider actually offers an endpoint for.
  // When the provider changes (or its endpoints do), pin the type to an
  // available one so the saved option never references a missing endpoint.
  useEffect(() => {
    if (!current || !hasSelectableType(current.agent.id)) return;
    const available = availableTypes(selectedProvider);
    if (available.length === 0) return;
    const id = current.agent.id as "kimi" | "opencode";
    const currentType = options[id].provider_type as "openai" | "anthropic";
    if (!available.includes(currentType)) {
      setOptions((o) => ({ ...o, [id]: { ...o[id], provider_type: available[0] } }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedProvider, current]);

  const configuredCount = configs?.filter((c) => c.configured).length ?? 0;

  const filteredProviders = useMemo(() => {
    if (!providers || !current) return [];
    const q = query.trim();
    return providers
      .map((p) => ({
        provider: p,
        models: q
          ? p.models.filter((m) => fuzzyMatch(q, `${p.name} ${p.key} ${m.id} ${m.name}`))
          : p.models,
      }))
      .filter(({ provider, models }) => models.length > 0 || fuzzyMatch(q, `${provider.name} ${provider.key}`));
  }, [providers, current, query]);

  function selectAgent(id: string) {
    setAgentId(id);
  }

  function selectModel(provider: Provider, model: ModelInfo) {
    if (!current || !providerCompatible(current, provider)) return;
    setProviderKey(provider.key);
    setModelId(model.id);
    setPickerOpen(false);
  }

  async function onSave() {
    if (!current || !providerKey || !modelId) return;
    setBusy(true);
    try {
      const saved = await setAgentModelConfig(current.agent.id, providerKey, modelId, options);
      setConfigs((prev) => prev?.map((c) => (c.agent.id === saved.agent.id ? saved : c)) ?? prev);
      toast.success(`已写入 ${current.agent.name} 模型配置`);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onSync() {
    if (!current) return;
    setBusy(true);
    try {
      const saved = await syncAgentModelConfig(current.agent.id);
      setConfigs((prev) => prev?.map((c) => (c.agent.id === saved.agent.id ? saved : c)) ?? prev);
      toast.success(`已同步 ${current.agent.name}`);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onSyncAll() {
    setBusy(true);
    try {
      const results = await syncAllAgentModelConfigs();
      const failed = results.filter((r) => !r.ok);
      if (failed.length) {
        toast.error(`同步完成，${failed.length} 个失败：${failed[0].message}`);
      } else {
        toast.success(`已同步 ${results.length} 个 Agent`);
      }
      await refresh(agentId ?? undefined);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <TooltipProvider>
    <>
      <PageHeader title="Agent 模型">
        为各 Agent 选用模型中心已保存的 provider 与密钥 · 自动生成并合并写入各 Agent 原生配置文件
      </PageHeader>

      <div className="border-border flex items-center gap-3 border-b px-6 py-3">
        <span className="text-muted-foreground text-[11.5px]">
          已配置 {configuredCount} / {configs?.length ?? 4}
        </span>
        <div className="bg-muted h-1.5 w-[120px] overflow-hidden rounded-full">
          <div
            className="bg-emerald-500 h-full rounded-full"
            style={{ width: `${configs?.length ? (configuredCount / configs.length) * 100 : 0}%` }}
          />
        </div>
        <div className="flex-1" />
        <Button variant="outline" size="sm" onClick={onSyncAll} disabled={busy || configuredCount === 0}>
          <IconRefresh data-icon="inline-start" />
          同步全部
        </Button>
        <Tooltip>
          <TooltipTrigger
            render={<span className="text-muted-foreground inline-flex cursor-help" />}
          >
            <IconHelp className="size-4" />
          </TooltipTrigger>
          <TooltipContent className="max-w-[280px] text-left leading-relaxed">
            不改 Agent 已保存的选择，按模型中心最新的端点 / 密钥 / 模型信息，重新生成并写入全部 Agent 的原生配置文件。常用于在模型中心改过端点、密钥或模型上下文窗口后批量刷新。
          </TooltipContent>
        </Tooltip>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-[260px_1fr] overflow-hidden max-[820px]:grid-cols-1 max-[820px]:grid-rows-[minmax(0,190px)_1fr]">
        <div className="border-border min-h-0 border-r max-[820px]:border-b max-[820px]:border-r-0">
          <div className="min-h-0 h-full overflow-auto p-2">
            {configs === null ? (
              <div className="flex flex-col gap-1.5">
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-[56px] rounded-lg" />
                ))}
              </div>
            ) : (
              configs.map((c) => (
                <button
                  key={c.agent.id}
                  onClick={() => selectAgent(c.agent.id)}
                  className={cn(
                    "flex w-full items-center gap-2.5 rounded-lg border border-transparent p-2.5 text-left transition-colors",
                    agentId === c.agent.id ? "bg-primary/10 border-primary/20" : "hover:bg-foreground/5",
                  )}
                >
                  <span
                    className="grid size-[32px] shrink-0 place-items-center rounded-lg text-[12px] font-bold text-white"
                    style={{ background: c.agent.color }}
                  >
                    {c.agent.name.replace(/[^A-Za-z一-龥]/g, "").slice(0, 2) || "··"}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-[13px] font-semibold">{c.agent.name}</span>
                    <span className="text-muted-foreground block truncate font-mono text-[11px]">
                      {c.configured ? `${c.provider} / ${c.model}` : "未配置"}
                    </span>
                  </span>
                  <span
                    title={c.configured ? "已配置" : "未配置"}
                    className={cn(
                      "size-2 shrink-0 rounded-full border",
                      c.configured ? "border-emerald-500 bg-emerald-500" : "border-border bg-transparent",
                    )}
                  />
                </button>
              ))
            )}
          </div>
        </div>

        <div className="min-h-0 overflow-auto px-4 pb-10 pt-4 sm:px-6">
          {!current ? (
            <Empty className="border-border mt-6">
              <EmptyHeader>
                <EmptyTitle>未选择 Agent</EmptyTitle>
                <EmptyDescription>从左侧选择一个 Agent 来配置模型。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-3">
                <div
                  className="grid size-[42px] shrink-0 place-items-center rounded-[11px] text-[17px] font-bold text-white"
                  style={{ background: current.agent.color }}
                >
                  {current.agent.name.replace(/[^A-Za-z一-龥]/g, "").slice(0, 2)}
                </div>
                <div className="min-w-0">
                  <div className="font-heading flex items-center gap-2 text-[19px] font-bold tracking-tight">
                    {current.agent.name}
                    <span className="text-primary border-primary/20 bg-primary/10 rounded-md border px-2 py-0.5 text-[11px] font-semibold">
                      {current.endpoint_kind === "claude" ? "Claude 兼容" : "OpenAI 兼容"}
                    </span>
                  </div>
                  <div className="text-muted-foreground mt-0.5 font-mono text-[11.5px]">
                    <div className="truncate">{current.config_path}</div>
                    {current.secondary_config_path && (
                      <div className="truncate">{current.secondary_config_path}</div>
                    )}
                  </div>
                </div>
                <div className="ml-auto flex items-center gap-1.5">
                  <Button variant="outline" size="sm" onClick={onSync} disabled={busy || !current.configured}>
                    <IconRefresh data-icon="inline-start" />
                    同步
                  </Button>
                  <Tooltip>
                    <TooltipTrigger
                      render={<span className="text-muted-foreground inline-flex cursor-help" />}
                    >
                      <IconHelp className="size-4" />
                    </TooltipTrigger>
                    <TooltipContent className="max-w-[280px] text-left leading-relaxed">
                      不改当前 Agent 已保存的选择，按模型中心最新的端点 / 密钥 / 模型信息，重新生成并写入该 Agent 的原生配置文件。常用于在模型中心改过端点、密钥或模型上下文窗口后刷新。
                    </TooltipContent>
                  </Tooltip>
                </div>
              </div>

              {current.error && (
                <div className="border-destructive/30 bg-destructive/10 text-destructive mt-4 rounded-lg border px-3 py-2 text-[12px]">
                  {current.error}，请重新选择模型。
                </div>
              )}

              <SectionTitle hint="来自模型中心，端点与密钥自动带出">选用模型</SectionTitle>
              <div className="bg-muted/60 border-border rounded-[10px] border">
                <button
                  onClick={() => setPickerOpen((v) => !v)}
                  className="hover:bg-foreground/[0.03] flex w-full items-center gap-3 p-3 text-left transition-colors"
                >
                  <span
                    className="grid size-[34px] shrink-0 place-items-center rounded-lg text-[12px] font-bold text-white"
                    style={{ background: selectedProvider?.color ?? "var(--primary)" }}
                  >
                    {selectedProvider?.initials ?? "?"}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2 text-[13.5px] font-semibold">
                      {modelLabel(selectedModel)}
                      {selectedModel?.context_window != null && (
                        <span className="bg-background text-muted-foreground rounded-md px-1.5 py-0.5 font-mono text-[10.5px] font-medium">
                          {formatTokens(selectedModel.context_window)}
                        </span>
                      )}
                    </span>
                    <span className="text-muted-foreground mt-0.5 block text-[11.5px]">
                      {selectedProvider ? `来自 ${selectedProvider.name}（${selectedProvider.key}）· 模型中心` : "请选择模型中心中的模型"}
                    </span>
                  </span>
                  <IconChevronDown className="text-muted-foreground size-4" />
                </button>
              </div>

              {pickerOpen && (
                <div className="border-border bg-popover mt-2 max-w-[520px] overflow-hidden rounded-[10px] border shadow-lg">
                  <div className="border-border relative border-b p-2">
                    <IconSearch className="text-muted-foreground absolute left-5 top-1/2 size-[14px] -translate-y-1/2" />
                    <Input
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                      placeholder="搜索 provider / 模型…"
                      className="h-[32px] pl-8"
                    />
                  </div>
                  <div className="max-h-[300px] overflow-auto p-1.5">
                    {filteredProviders.length === 0 ? (
                      <div className="text-muted-foreground px-2 py-6 text-center text-[12px]">没有匹配的模型</div>
                    ) : (
                      filteredProviders.map(({ provider, models }) => {
                        const compatible = providerCompatible(current, provider);
                        return (
                          <div key={provider.key}>
                            <div className="text-muted-foreground px-2 py-1.5 text-[10px] font-bold uppercase tracking-wider">
                              {provider.name} · {provider.key}
                            </div>
                            {models.map((m) => (
                              <button
                                key={`${provider.key}/${m.id}`}
                                onClick={() => selectModel(provider, m)}
                                disabled={!compatible}
                                className={cn(
                                  "flex w-full items-center gap-2 rounded-md px-2 py-2 text-left transition-colors",
                                  compatible ? "hover:bg-primary/10" : "cursor-not-allowed opacity-50",
                                  providerKey === provider.key && modelId === m.id && "bg-primary/10",
                                )}
                                title={compatible ? undefined : `该 Provider 缺少 ${current.endpoint_kind === "claude" ? "Claude" : "OpenAI"} 兼容端点`}
                              >
                                <span
                                  className="grid size-[26px] shrink-0 place-items-center rounded-md text-[10px] font-bold text-white"
                                  style={{ background: provider.color }}
                                >
                                  {provider.initials}
                                </span>
                                <span className="min-w-0 flex-1">
                                  <span className="block truncate font-mono text-[12.5px] font-semibold">{m.id}</span>
                                  <span className="text-muted-foreground block truncate text-[11px]">
                                    {compatible
                                      ? `${provider.name} · ${providerEndpointLabel(current, provider)}`
                                      : `缺少 ${current.endpoint_kind === "claude" ? "Claude" : "OpenAI"} 兼容端点`}
                                  </span>
                                </span>
                                <span className="text-muted-foreground shrink-0 font-mono text-[10.5px]">
                                  {compatible ? formatTokens(m.context_window) : "不可用"}
                                </span>
                                {providerKey === provider.key && modelId === m.id && (
                                  <IconCheck className="text-primary size-[15px] shrink-0" />
                                )}
                              </button>
                            ))}
                          </div>
                        );
                      })
                    )}
                  </div>
                </div>
              )}

              <AgentSpecificOptions
                current={current}
                providerKey={providerKey}
                modelId={modelId}
                selectedModel={selectedModel}
                selectedProvider={selectedProvider}
                options={options}
                setOptions={setOptions}
              />

              <SectionTitle badge="合并写入 · 保留原有字段">生成配置预览</SectionTitle>
              <div className="flex flex-col gap-2">
                {preview.length === 0 ? (
                  <div className="text-muted-foreground border-border rounded-lg border border-dashed py-6 text-center text-[12.5px]">
                    选择可用模型后显示预览
                  </div>
                ) : (
                  preview.map((file) => (
                    <div key={file.path} className="border-border overflow-hidden rounded-[10px] border">
                      <div className="border-border bg-muted/60 flex items-center gap-2 border-b px-3 py-2">
                        <span className="text-[11.5px] font-semibold">配置文件</span>
                        <span className="text-muted-foreground truncate font-mono text-[11px]">{file.path}</span>
                      </div>
                      <pre className="bg-muted/30 overflow-auto p-3.5 text-[11.5px] leading-6"><code>{file.content}</code></pre>
                    </div>
                  ))
                )}
              </div>

              <div className="mt-4 flex justify-end gap-2">
                <Button variant="outline" onClick={() => current && selectAgent(current.agent.id)} disabled={busy}>
                  放弃
                </Button>
                <Button onClick={onSave} disabled={busy || !providerKey || !modelId}>
                  <IconCpu2 data-icon="inline-start" />
                  {busy ? "写入中…" : "写入配置"}
                </Button>
              </div>
            </>
          )}
        </div>
      </div>
    </>
    </TooltipProvider>
  );
}

function SectionTitle({ children, hint, badge }: { children: string; hint?: string; badge?: string }) {
  return (
    <div className="text-muted-foreground mt-6 flex items-center gap-2 px-0.5 pb-2 text-[10.5px] font-semibold uppercase tracking-wider">
      {children}
      {hint && <span className="font-medium normal-case tracking-normal">· {hint}</span>}
      {badge && (
        <span className="ml-auto rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-semibold text-emerald-600 normal-case tracking-normal">
          {badge}
        </span>
      )}
    </div>
  );
}

function AgentSpecificOptions({
  current,
  providerKey,
  modelId,
  selectedModel,
  selectedProvider,
  options,
  setOptions,
}: {
  current: AgentModelConfig;
  providerKey: string;
  modelId: string;
  selectedModel: ModelInfo | null;
  selectedProvider: Provider | null;
  options: AgentModelOptions;
  setOptions: (options: AgentModelOptions) => void;
}) {
  if (current.agent.id === "claude") {
    const enable1m = (selectedModel?.context_window ?? 0) >= 1_000_000;
    const selected = modelId
      ? enable1m
        ? `${modelId}${modelId.endsWith("[1m]") ? "" : "[1m]"}`
        : modelId
      : "";
    const slotModels = selectedProvider?.models ?? [];
    const slotModelById = (id: string) => slotModels.find((m) => m.id === id) ?? null;
    const with1mIf = (id: string) => {
      const m = slotModelById(id);
      const oneM = m ? (m.context_window ?? 0) >= 1_000_000 : false;
      return oneM && !id.endsWith("[1m]") ? `${id}[1m]` : id;
    };
    const slots = [
      { label: "Haiku", key: "haiku_model" as const },
      { label: "Sonnet", key: "sonnet_model" as const },
      { label: "Opus", key: "opus_model" as const },
    ];
    const same = options.claude.same_model_for_slots;
    return (
      <>
        <SectionTitle badge="settings.json · env">Claude 专属</SectionTitle>
        <SwitchRow
          title="三个默认槽使用同一模型"
          description={
            same
              ? `Haiku / Sonnet / Opus 均写入 ${selected || "所选模型"}`
              : "分别为 Haiku / Sonnet / Opus 选择当前 provider 下的模型"
          }
          checked={same}
          onChange={(v) => setOptions({ ...options, claude: { ...options.claude, same_model_for_slots: v } })}
        />
        <div className="mt-2 flex flex-col gap-1.5">
          {slots.map(({ label, key }) => {
            const raw = options.claude[key];
            const shown = same ? selected : raw ? with1mIf(raw) : selected || "—";
            const valueInList = raw && slotModels.some((m) => m.id === raw);
            const selectValue = valueInList
              ? raw
              : modelId && slotModels.some((m) => m.id === modelId)
                ? modelId
                : "";
            return (
              <div key={label} className="bg-muted/60 border-border flex items-center gap-2 rounded-lg border px-3 py-2">
                <span className="bg-background text-muted-foreground w-[64px] shrink-0 rounded-md border px-2 py-0.5 text-center text-[10.5px] font-semibold">
                  {label}
                </span>
                {same || slotModels.length === 0 ? (
                  <span className="font-mono text-[12.5px] font-semibold">{shown}</span>
                ) : (
                  <select
                    value={selectValue}
                    onChange={(e) =>
                      setOptions({ ...options, claude: { ...options.claude, [key]: e.target.value } })
                    }
                    className="border-border bg-background h-8 min-w-0 flex-1 rounded-md border px-2 font-mono text-[12px]"
                  >
                    {slotModels.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.id}
                        {m.name ? ` · ${m.name}` : ""}
                        {(m.context_window ?? 0) >= 1_000_000 ? "  [1m]" : ""}
                      </option>
                    ))}
                  </select>
                )}
              </div>
            );
          })}
        </div>
      </>
    );
  }

  if (current.agent.id === "kimi") {
    const types = availableTypes(selectedProvider);
    const hasProvider = Boolean(selectedProvider);
    const ptype = types.includes(options.kimi.provider_type as "openai" | "anthropic")
      ? options.kimi.provider_type
      : types[0] ?? options.kimi.provider_type;
    const baseUrl =
      ptype === "anthropic"
        ? selectedProvider?.endpoint_claude ?? ""
        : selectedProvider?.endpoint_openai ?? "";
    const singleType = !hasProvider || types.length <= 1;
    return (
      <>
        <SectionTitle badge="config.toml">Kimi 专属</SectionTitle>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1">
            <span className="text-muted-foreground text-[11.5px] font-medium">模型提供商</span>
            {singleType ? (
              <div className="border-border bg-muted text-muted-foreground flex h-9 items-center rounded-md border px-3 text-[13px]">
                {hasProvider ? typeLabel(ptype) : "—"}
              </div>
            ) : (
              <select
                value={ptype}
                onChange={(e) => setOptions({ ...options, kimi: { ...options.kimi, provider_type: e.target.value } })}
                className="border-border bg-muted h-9 rounded-md border px-3 text-[13px]"
              >
                {types.map((t) => (
                  <option key={t} value={t}>
                    {typeLabel(t)}
                  </option>
                ))}
              </select>
            )}
            <span className="text-muted-foreground text-[11px]">
              {hasProvider
                ? "仅显示当前提供商已配置端点的兼容类型"
                : "先在上方选择模型"}
              {hasProvider && ptype && (
                <span> · base_url：<Mono>{baseUrl || "未配置"}</Mono></span>
              )}
            </span>
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-muted-foreground text-[11.5px] font-medium">模型能力</span>
            <Input
              value={options.kimi.capabilities.join(", ")}
              onChange={(e) =>
                setOptions({
                  ...options,
                  kimi: { ...options.kimi, capabilities: e.target.value.split(",").map((s) => s.trim()).filter(Boolean) },
                })
              }
              className="h-9 px-3 font-mono text-[13px]"
            />
          </label>
        </div>
      </>
    );
  }

  if (current.agent.id === "opencode") {
    const types = availableTypes(selectedProvider);
    const hasProvider = Boolean(selectedProvider);
    const ptype = types.includes(options.opencode.provider_type as "openai" | "anthropic")
      ? options.opencode.provider_type
      : types[0] ?? options.opencode.provider_type;
    const npm = ptype === "anthropic" ? "@ai-sdk/anthropic" : "@ai-sdk/openai-compatible";
    const baseUrl =
      ptype === "anthropic"
        ? selectedProvider?.endpoint_claude ?? ""
        : selectedProvider?.endpoint_openai ?? "";
    const singleType = !hasProvider || types.length <= 1;
    return (
      <>
        <SectionTitle badge="opencode.jsonc">Opencode 专属</SectionTitle>
        <label className="flex flex-col gap-1">
          <span className="text-muted-foreground text-[11.5px] font-medium">模型提供商</span>
          {singleType ? (
            <div className="border-border bg-muted text-muted-foreground flex h-9 items-center rounded-md border px-3 text-[13px]">
              {hasProvider ? typeLabel(ptype) : "—"}
            </div>
          ) : (
            <select
              value={ptype}
              onChange={(e) => setOptions({ ...options, opencode: { ...options.opencode, provider_type: e.target.value } })}
              className="border-border bg-muted h-9 rounded-md border px-3 text-[13px]"
            >
              {types.map((t) => (
                <option key={t} value={t}>
                  {typeLabel(t)}
                </option>
              ))}
            </select>
          )}
          <span className="text-muted-foreground text-[11px]">
            {hasProvider
              ? "仅显示当前提供商已配置端点的兼容类型"
              : "先在上方选择模型"}
            {hasProvider && ptype && (
              <span>
                {" · npm："}<Mono>{npm}</Mono>
                {" · baseURL："}<Mono>{baseUrl || "未配置"}</Mono>
              </span>
            )}
          </span>
        </label>
      </>
    );
  }

  if (current.agent.id === "codex") {
    return (
      <>
        <SectionTitle badge="config.toml">Codex 专属</SectionTitle>
        <label className="flex flex-col gap-1">
          <span className="text-muted-foreground text-[11.5px] font-medium">env_key</span>
          <Input
            value={options.codex.env_key}
            onChange={(e) => setOptions({ ...options, codex: { ...options.codex, env_key: e.target.value } })}
            placeholder={providerKey ? defaultEnvKey(providerKey) : "BYTEDANCE_MODEL_API_KEY"}
            className="h-9 px-3 font-mono text-[13px]"
          />
          <span className="text-muted-foreground text-[11px]">
            只写入 env_key，环境变量由用户自行配置 · wire_api 固定为 responses
          </span>
        </label>
      </>
    );
  }

  return null;
}

function SwitchRow({
  title,
  description,
  checked,
  onChange,
}: {
  title: string;
  description: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <button
      onClick={() => onChange(!checked)}
      className="bg-muted/60 border-border mb-2 flex w-full items-start gap-3 rounded-lg border px-3 py-2.5 text-left"
    >
      <span className="min-w-0 flex-1">
        <span className="block text-[12.5px] font-semibold">{title}</span>
        <span className="text-muted-foreground mt-0.5 block text-[11.5px]">{description}</span>
      </span>
      <span className={cn("relative h-[22px] w-[38px] shrink-0 rounded-full transition-colors", checked ? "bg-primary" : "bg-muted-foreground/25")}>
        <span
          className={cn(
            "absolute top-0.5 size-[18px] rounded-full bg-white transition-transform",
            checked ? "translate-x-[18px]" : "translate-x-0.5",
          )}
        />
      </span>
    </button>
  );
}
