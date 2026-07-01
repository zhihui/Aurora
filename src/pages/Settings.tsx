import { useEffect, useState } from "react";
import { IconFolderOpen, IconFolderPlus, IconKey } from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader, Mono } from "@/components/PageHeader";
import { cn } from "@/lib/utils";
import {
  listAgentDirs,
  createAgentDir,
  openPath,
  getLlmConfig,
  setLlmConfig,
  type AgentDirInfo,
} from "@/lib/api";

export function Settings() {
  const [dirs, setDirs] = useState<AgentDirInfo[] | null>(null);

  // ── LLM config ──
  const [endpoint, setEndpoint] = useState("");
  const [model, setModel] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [hasKey, setHasKey] = useState(false);
  const [llmLoaded, setLlmLoaded] = useState(false);
  const [savingLlm, setSavingLlm] = useState(false);

  async function refresh() {
    try {
      setDirs(await listAgentDirs());
    } catch (e) {
      toast.error(String(e));
    }
  }
  async function loadLlm() {
    try {
      const c = await getLlmConfig();
      setEndpoint(c.endpoint);
      setModel(c.model);
      setHasKey(c.has_key);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setLlmLoaded(true);
    }
  }
  useEffect(() => {
    refresh();
    loadLlm();
  }, []);

  async function onSaveLlm() {
    setSavingLlm(true);
    try {
      // Empty key field → keep the stored key (send null).
      await setLlmConfig(endpoint, model, apiKey.trim() === "" ? null : apiKey);
      setApiKey("");
      toast.success("已保存模型配置");
      await loadLlm();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSavingLlm(false);
    }
  }

  async function onCreate(id: string) {
    try {
      await createAgentDir(id);
      toast.success("已创建目录");
      await refresh();
    } catch (e) {
      toast.error(String(e));
    }
  }

  function initials(name: string) {
    return name.replace(/[^A-Za-z一-龥]/g, "").slice(0, 2) || "··";
  }

  return (
    <>
      <PageHeader title="设置">
        各 agent 的技能目录位置 · Aurora 数据存于 <Mono>~/.aurora/</Mono>
      </PageHeader>

      <div className="flex-1 overflow-auto px-4 pb-6 pt-4">
        <div className="text-muted-foreground px-2 pb-1 text-[10.5px] font-semibold uppercase tracking-wider">
          Agent 技能目录
        </div>

        <div className="flex flex-col gap-2">
          {dirs === null
            ? Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-[64px] rounded-[10px]" />
              ))
            : dirs.map((d) => (
                <div key={d.id} className="border-border flex items-center gap-3.5 rounded-[10px] border p-3.5">
                  <div
                    className="grid size-[34px] shrink-0 place-items-center rounded-[9px] text-[13px] font-bold text-white"
                    style={{ background: d.color }}
                  >
                    {initials(d.name)}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-semibold">{d.name}</div>
                    <div className="text-muted-foreground mt-0.5 truncate font-mono text-[12px]">
                      {d.path}
                    </div>
                  </div>
                  <span
                    className={cn(
                      "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-semibold",
                      d.exists
                        ? "bg-emerald-500/10 text-emerald-600"
                        : "bg-muted text-muted-foreground border-border border",
                    )}
                  >
                    {d.exists ? (
                      <>
                        <span className="size-1.5 rounded-full bg-emerald-500" />
                        已就绪 · {d.skill_count} 个技能
                      </>
                    ) : (
                      "目录不存在"
                    )}
                  </span>
                  {d.exists ? (
                    <Button variant="outline" size="sm" onClick={() => openPath(d.path)}>
                      <IconFolderOpen data-icon="inline-start" />
                      打开
                    </Button>
                  ) : (
                    <Button variant="outline" size="sm" onClick={() => onCreate(d.id)}>
                      <IconFolderPlus data-icon="inline-start" />
                      创建目录
                    </Button>
                  )}
                </div>
              ))}
        </div>

        <div className="text-muted-foreground px-2 pb-1 pt-5 text-[10.5px] font-semibold uppercase tracking-wider">
          数据目录
        </div>
        <div className="border-border flex items-center gap-3.5 rounded-[10px] border p-3.5">
          <div className="bg-primary grid size-[34px] shrink-0 place-items-center rounded-[9px] text-[13px] font-bold text-white">
            AU
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold">Aurora 数据</div>
            <div className="text-muted-foreground mt-0.5 truncate font-mono text-[12px]">
              ~/.aurora/ · skills/ · packs.json · config.json
            </div>
          </div>
          <Button variant="outline" size="sm" onClick={() => openPath("~/.aurora")}>
            <IconFolderOpen data-icon="inline-start" />
            打开
          </Button>
        </div>

        <div className="text-muted-foreground px-2 pb-1 pt-5 text-[10.5px] font-semibold uppercase tracking-wider select-none">
          大语言模型
        </div>
        <div className="border-border flex flex-col gap-3 rounded-[10px] border p-4">
          <div className="flex items-start gap-3.5">
            <div className="bg-primary/10 text-primary grid size-[34px] shrink-0 place-items-center rounded-[9px]">
              <IconKey className="size-[18px]" stroke={1.8} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-sm font-semibold select-none">翻译模型</div>
              <div className="text-muted-foreground mt-0.5 text-[12px] select-none">
                兼容 OpenAI Chat Completions 接口（OpenAI / DeepSeek / Kimi 等），用于翻译 SKILL.md
              </div>
            </div>
          </div>

          {!llmLoaded ? (
            <Skeleton className="h-[148px] rounded-[8px]" />
          ) : (
            <div className="flex flex-col gap-2.5">
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium select-none">
                  端点 Endpoint
                </span>
                <Input
                  value={endpoint}
                  onChange={(e) => setEndpoint(e.target.value)}
                  placeholder="https://api.openai.com/v1"
                  className="font-mono text-[12.5px]"
                  spellCheck={false}
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium select-none">
                  模型名称 Model
                </span>
                <Input
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  placeholder="gpt-4o-mini"
                  className="font-mono text-[12.5px]"
                  spellCheck={false}
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-muted-foreground text-[11.5px] font-medium select-none">
                  API 密钥
                  <span className="text-muted-foreground/70 ml-1.5 font-normal">
                    仅可保存，不可查看
                  </span>
                </span>
                <Input
                  type="password"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder={hasKey ? "已保存密钥 · 留空则保持不变" : "sk-…"}
                  className="font-mono text-[12.5px]"
                  autoComplete="off"
                  spellCheck={false}
                />
              </label>
              <div className="flex items-center justify-between pt-0.5">
                <span
                  className={cn(
                    "inline-flex items-center gap-1.5 text-[11.5px] font-medium select-none",
                    hasKey ? "text-emerald-600" : "text-muted-foreground",
                  )}
                >
                  <span
                    className={cn(
                      "size-1.5 rounded-full",
                      hasKey ? "bg-emerald-500" : "bg-muted-foreground/40",
                    )}
                  />
                  {hasKey ? "密钥已保存" : "尚未保存密钥"}
                </span>
                <Button size="sm" onClick={onSaveLlm} disabled={savingLlm}>
                  {savingLlm ? "保存中…" : "保存配置"}
                </Button>
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
