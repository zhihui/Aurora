import { useState } from "react";
import {
  IconBrandGithub,
  IconLink,
  IconFolder,
  IconClipboard,
  IconTerminal,
  IconLoader2,
  IconCheck,
  IconAlertTriangle,
} from "@tabler/icons-react";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  parseGithubImport,
  parseUrlImport,
  parseLocalImport,
  importFromStaging,
  cancelImport,
  createSkillText,
  type ParsedImport,
} from "@/lib/api";

type Source = "github" | "cmd" | "url" | "local" | "paste";
type Sel = { selected: boolean; name: string };

/// Parse a GitHub input that may be a raw URL, an `owner/repo` shorthand, or a
/// `npx skills add ... --skill <name>` command line. Returns the cleaned repo
/// reference (URL or owner/repo) to send to the backend, plus an optional skill
/// name filter requested via `--skill`.
///
/// Recognized forms:
///   https://github.com/owner/repo[/tree/<branch>/<subpath>]
///   owner/repo
///   npx skills add <either-of-above> [--skill <name>]
///   npx skills add <either-of-above> [--skill=<name>]
function parseGithubInput(raw: string): { ref: string; skill: string | null } {
  const s = raw.trim();
  if (!s) return { ref: "", skill: null };

  // Extract --skill <name>  or  --skill=<name>  (anywhere in the input).
  let skill: string | null = null;
  const eq = s.match(/--skill=(\S+)/);
  const sp = s.match(/--skill\s+(\S+)/);
  const m = eq || sp;
  if (m) skill = m[1];

  // Strip an optional `npx skills add` (and any `npx <pkg> <add|install>`) prefix.
  let rest = s.replace(/^npx\s+\S+(?:\s+(?:add|install))?\s+/i, "").trim();

  // Drop any remaining flags / extra tokens after the first whitespace.
  // The repo reference itself contains no spaces, so the first token is it.
  rest = rest.split(/\s+/)[0] ?? "";

  // Remove a trailing .git for cleanliness (backend tolerates it too).
  if (rest.endsWith(".git")) rest = rest.slice(0, -4);

  return { ref: rest, skill };
}

const SOURCES: { id: Source; label: string; sub: string; icon: typeof IconLink }[] = [
  { id: "github", label: "GitHub", sub: "仓库 / 子目录", icon: IconBrandGithub },
  { id: "cmd", label: "命令行", sub: "npx skills add", icon: IconTerminal },
  { id: "url", label: "链接 URL", sub: "SKILL.md / zip", icon: IconLink },
  { id: "local", label: "本地", sub: "文件夹 / 压缩包", icon: IconFolder },
  { id: "paste", label: "粘贴 / 新建", sub: "直接写 SKILL.md", icon: IconClipboard },
];

export function ImportSkillDialog({
  open,
  onOpenChange,
  onImported,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  onImported: () => void;
}) {
  const [source, setSource] = useState<Source>("github");

  // source inputs
  const [ghLink, setGhLink] = useState("");
  const [ghBranch, setGhBranch] = useState("");
  const [cmdInput, setCmdInput] = useState("");
  const [url, setUrl] = useState("");
  const [localPath, setLocalPath] = useState("");
  const [pasteName, setPasteName] = useState("");
  const [pasteContent, setPasteContent] = useState("");

  // parse result + selections
  const [parsed, setParsed] = useState<ParsedImport | null>(null);
  const [sels, setSels] = useState<Sel[]>([]);
  const [busy, setBusy] = useState(false);

  function resetParsed() {
    // Drop any temp staging the previous parse created.
    if (parsed?.is_temp) cancelImport(parsed.root, parsed.is_temp).catch(() => {});
    setParsed(null);
    setSels([]);
  }

  function close(o: boolean) {
    if (!o) {
      resetParsed();
      setSource("github");
      setGhLink("");
      setGhBranch("");
      setCmdInput("");
      setUrl("");
      setLocalPath("");
      setPasteName("");
      setPasteContent("");
    }
    onOpenChange(o);
  }

  function switchSource(s: Source) {
    if (s === source) return;
    resetParsed();
    setSource(s);
  }

  function initSelections(p: ParsedImport) {
    setSels(
      p.skills.map((s) => ({
        selected: !s.exists, // collisions start unchecked until renamed
        name: s.exists ? `${s.name}-2` : s.name,
      })),
    );
  }

  async function runParse() {
    setBusy(true);
    resetParsed();
    try {
      let p: ParsedImport;
      if (source === "github") {
        if (!ghLink.trim()) throw "请输入 GitHub 链接";
        p = await parseGithubImport(ghLink.trim(), ghBranch.trim() || null);
      } else if (source === "cmd") {
        const { ref, skill } = parseGithubInput(cmdInput);
        if (!ref) throw "请输入 skills add 命令";
        p = await parseGithubImport(ref, null);
        if (skill) {
          // `--skill <name>` requested: keep only the matching skill.
          p = { ...p, skills: p.skills.filter((s) => s.name === skill) };
          if (p.skills.length === 0) {
            throw `仓库中未找到技能：${skill}`;
          }
        }
      } else if (source === "url") {
        if (!url.trim()) throw "请输入链接";
        p = await parseUrlImport(url.trim());
      } else {
        if (!localPath.trim()) throw "请选择文件夹或压缩包";
        p = await parseLocalImport(localPath.trim());
      }
      setParsed(p);
      initSelections(p);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function browse(directory: boolean) {
    try {
      const picked = await openDialog({
        multiple: false,
        directory,
        filters: directory
          ? undefined
          : [{ name: "技能压缩包", extensions: ["zip", "gz", "tgz"] }],
      });
      if (typeof picked === "string") {
        setLocalPath(picked);
        resetParsed();
      }
    } catch (e) {
      toast.error(String(e));
    }
  }

  async function runImport() {
    if (!parsed) return;
    const selections = parsed.skills
      .map((s, i) => ({ rel_path: s.rel_path, name: sels[i].name.trim(), on: sels[i].selected }))
      .filter((x) => x.on)
      .map(({ rel_path, name }) => ({ rel_path, name }));
    if (selections.length === 0) {
      toast.error("请至少选择一个技能");
      return;
    }
    setBusy(true);
    try {
      const res = await importFromStaging(parsed.root, parsed.is_temp, selections);
      if (res.imported.length > 0) {
        toast.success(`已导入 ${res.imported.length} 个技能`);
        onImported();
      }
      if (res.errors.length > 0) {
        toast.error(res.errors.join("；"));
      }
      // staging is consumed by the backend on import; avoid double-cancel.
      setParsed(null);
      setSels([]);
      if (res.errors.length === 0) close(false);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function runCreate() {
    if (!pasteName.trim()) {
      toast.error("请输入技能名称");
      return;
    }
    setBusy(true);
    try {
      await createSkillText(pasteName.trim(), pasteContent);
      toast.success("已创建技能");
      onImported();
      close(false);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  }

  const selectedCount = sels.filter((s) => s.selected).length;

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="flex h-[560px] max-h-[90vh] w-[94vw] flex-col gap-0 overflow-hidden p-0 sm:max-w-[640px]">
        <DialogHeader className="border-border border-b px-5 py-4 pr-12">
          <DialogTitle>导入技能</DialogTitle>
          <DialogDescription className="font-mono text-[11.5px]">
            从外部来源复制技能到 ~/.aurora/skills/
          </DialogDescription>
        </DialogHeader>

        <div className="grid min-h-0 flex-1 grid-cols-[164px_1fr] overflow-hidden">
          {/* source rail */}
          <div className="bg-muted/40 border-border flex flex-col gap-0.5 border-r p-2.5 select-none">
            {SOURCES.map(({ id, label, sub, icon: Icon }) => (
              <button
                key={id}
                onClick={() => switchSource(id)}
                className={cn(
                  "flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-left transition-colors",
                  source === id
                    ? "bg-primary text-primary-foreground shadow-sm"
                    : "text-foreground hover:bg-foreground/[0.05]",
                )}
              >
                <Icon className="size-[17px] shrink-0" stroke={1.8} />
                <span className="min-w-0">
                  <span className="block text-[12.5px] font-medium leading-tight">{label}</span>
                  <span
                    className={cn(
                      "block text-[10.5px] leading-tight",
                      source === id ? "text-primary-foreground/80" : "text-muted-foreground",
                    )}
                  >
                    {sub}
                  </span>
                </span>
              </button>
            ))}
          </div>

          {/* form + result */}
          <div className="flex min-h-0 flex-col overflow-auto">
            <div
              className={cn(
                "flex flex-col gap-3.5 p-5",
                source === "paste" && "min-h-0 flex-1",
              )}
            >
              {source === "github" && (
                <>
                  <Field label="仓库或子目录链接">
                    <Input
                      value={ghLink}
                      onChange={(e) => setGhLink(e.target.value)}
                      placeholder="https://github.com/owner/repo/tree/main/skills/pdf"
                      className="font-mono text-[12px]"
                      spellCheck={false}
                    />
                  </Field>
                  <div className="flex items-end gap-2.5">
                    <Field label="分支" hint="留空自动取默认分支" className="flex-1">
                      <Input
                        value={ghBranch}
                        onChange={(e) => setGhBranch(e.target.value)}
                        placeholder="main"
                        className="font-mono text-[12px]"
                        spellCheck={false}
                      />
                    </Field>
                    <Button onClick={runParse} disabled={busy}>
                      {busy ? <IconLoader2 className="animate-spin" data-icon="inline-start" /> : null}
                      解析
                    </Button>
                  </div>
                  <p className="text-muted-foreground text-[11px] leading-relaxed select-none">
                    支持 <code className="font-mono">owner/repo</code> ·{" "}
                    <code className="font-mono">.../tree/&lt;分支&gt;/&lt;子目录&gt;</code> ·{" "}
                    <code className="font-mono">.git</code> 地址。解析后列出含 SKILL.md 的目录供勾选。
                  </p>
                </>
              )}

              {source === "cmd" && (
                <>
                  <p className="text-muted-foreground text-[11px] leading-relaxed select-none">
                    粘贴 <code className="font-mono">npx skills add &lt;repo&gt; [--skill &lt;name&gt;]</code>{" "}
                    或直接 <code className="font-mono">owner/repo</code>。带{" "}
                    <code className="font-mono">--skill</code> 时只导入该指定技能。
                  </p>
                  <Field label="skills add 命令">
                    <textarea
                      value={cmdInput}
                      onChange={(e) => setCmdInput(e.target.value)}
                      placeholder="npx skills add obra/superpowers --skill brainstorming"
                      spellCheck={false}
                      className="border-input bg-background focus-visible:ring-ring h-[88px] w-full resize-none rounded-md border px-3 py-2 font-mono text-[12px] leading-relaxed focus-visible:outline-none focus-visible:ring-2"
                    />
                  </Field>
                  <div className="flex justify-end">
                    <Button onClick={runParse} disabled={busy}>
                      {busy ? <IconLoader2 className="animate-spin" data-icon="inline-start" /> : null}
                      解析
                    </Button>
                  </div>
                </>
              )}

              {source === "url" && (
                <>
                  <Field label="文件链接">
                    <Input
                      value={url}
                      onChange={(e) => setUrl(e.target.value)}
                      placeholder="https://example.com/skill/SKILL.md 或 .../skill.zip"
                      className="font-mono text-[12px]"
                      spellCheck={false}
                    />
                  </Field>
                  <div className="flex justify-end">
                    <Button onClick={runParse} disabled={busy}>
                      {busy ? <IconLoader2 className="animate-spin" data-icon="inline-start" /> : null}
                      解析
                    </Button>
                  </div>
                  <p className="text-muted-foreground text-[11px] leading-relaxed select-none">
                    指向 SKILL.md → 作为单文件技能；指向 .zip / .tar.gz → 下载解压并定位 SKILL.md。
                  </p>
                </>
              )}

              {source === "local" && (
                <>
                  <Field label="文件夹或压缩包路径">
                    <Input
                      value={localPath}
                      onChange={(e) => setLocalPath(e.target.value)}
                      placeholder="/path/to/skill 或 /path/to/skill.zip"
                      className="font-mono text-[12px]"
                      spellCheck={false}
                    />
                  </Field>
                  <div className="flex items-center gap-2.5">
                    <Button variant="outline" size="sm" onClick={() => browse(true)}>
                      <IconFolder data-icon="inline-start" />
                      选文件夹
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => browse(false)}>
                      <IconFolder data-icon="inline-start" />
                      选压缩包
                    </Button>
                    <div className="flex-1" />
                    <Button onClick={runParse} disabled={busy}>
                      {busy ? <IconLoader2 className="animate-spin" data-icon="inline-start" /> : null}
                      解析
                    </Button>
                  </div>
                  <p className="text-muted-foreground text-[11px] leading-relaxed select-none">
                    内容会被复制进技能中心，原文件保持不变。
                  </p>
                </>
              )}

              {source === "paste" && (
                <>
                  <Field label="技能名称">
                    <Input
                      value={pasteName}
                      onChange={(e) => setPasteName(e.target.value)}
                      placeholder="pdf-extract"
                      className="font-mono text-[12.5px]"
                      spellCheck={false}
                    />
                  </Field>
                  <Field label="SKILL.md 内容" hint="留空将生成模板" className="min-h-0 flex-1">
                    <textarea
                      value={pasteContent}
                      onChange={(e) => setPasteContent(e.target.value)}
                      placeholder={"---\nname: pdf-extract\ndescription: …\n---\n\n# pdf-extract\n…"}
                      spellCheck={false}
                      className="border-input bg-background focus-visible:ring-ring h-full min-h-[120px] w-full resize-none rounded-md border px-3 py-2 font-mono text-[12px] leading-relaxed focus-visible:outline-none focus-visible:ring-2"
                    />
                  </Field>
                </>
              )}
            </div>

            {/* parsed result */}
            {parsed && (
              <div className="border-border bg-muted/30 mt-auto border-t px-5 py-4">
                <div className="text-muted-foreground mb-2.5 flex items-center gap-1.5 text-[11.5px] font-medium select-none">
                  <IconCheck className="size-3.5 text-emerald-600" />
                  发现 {parsed.skills.length} 个技能 · 已选 {selectedCount} 个
                </div>
                <div className="flex flex-col gap-2">
                  {parsed.skills.map((s, i) => (
                    <div
                      key={s.rel_path + s.name}
                      className="border-border bg-card flex items-center gap-3 rounded-lg border p-2.5"
                    >
                      <button
                        onClick={() =>
                          setSels((prev) =>
                            prev.map((x, j) => (j === i ? { ...x, selected: !x.selected } : x)),
                          )
                        }
                        className={cn(
                          "grid size-[18px] shrink-0 place-items-center rounded-[5px] border transition-colors",
                          sels[i]?.selected
                            ? "bg-primary border-primary text-primary-foreground"
                            : "border-border bg-background",
                        )}
                      >
                        {sels[i]?.selected && <IconCheck className="size-3" stroke={3} />}
                      </button>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="truncate font-mono text-[12.5px] font-semibold">
                            {s.name}
                          </span>
                          {s.exists && (
                            <span className="inline-flex items-center gap-1 rounded-[5px] border border-amber-500/30 bg-amber-500/10 px-1.5 py-px text-[10px] font-semibold text-amber-600 select-none">
                              <IconAlertTriangle className="size-2.5" />
                              已存在
                            </span>
                          )}
                        </div>
                        {s.description && (
                          <div className="text-muted-foreground mt-0.5 truncate text-[11.5px]">
                            {s.description}
                          </div>
                        )}
                      </div>
                      <Input
                        value={sels[i]?.name ?? ""}
                        onChange={(e) =>
                          setSels((prev) =>
                            prev.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)),
                          )
                        }
                        title="导入后的名称"
                        className="h-8 w-[130px] shrink-0 font-mono text-[11.5px]"
                        spellCheck={false}
                      />
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>

        {/* shared footer */}
        <div className="border-border flex items-center gap-2.5 border-t px-5 py-3">
          <span className="text-muted-foreground text-[11px] select-none">
            {source === "paste"
              ? "在 ~/.aurora/skills/ 下创建技能"
              : "仅接受含 SKILL.md 的内容 · 复制到中心后再分配"}
          </span>
          <div className="flex-1" />
          <Button variant="outline" onClick={() => close(false)}>
            取消
          </Button>
          {source === "paste" ? (
            <Button onClick={runCreate} disabled={busy || !pasteName.trim()}>
              {busy ? <IconLoader2 className="animate-spin" data-icon="inline-start" /> : null}
              创建
            </Button>
          ) : (
            <Button onClick={runImport} disabled={busy || !parsed || selectedCount === 0}>
              {busy ? <IconLoader2 className="animate-spin" data-icon="inline-start" /> : null}
              导入{selectedCount > 0 ? ` ${selectedCount} 个技能` : ""}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  hint,
  className,
  children,
}: {
  label: string;
  hint?: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <label className={cn("flex flex-col gap-1.5", className)}>
      <span className="text-muted-foreground text-[11.5px] font-medium select-none">
        {label}
        {hint && <span className="text-muted-foreground/70 ml-1.5 font-normal">{hint}</span>}
      </span>
      {children}
    </label>
  );
}
