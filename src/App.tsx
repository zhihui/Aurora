import { useEffect, useMemo, useState } from "react";
import { Toaster } from "@/components/ui/sonner";
import { Sidebar, type View } from "@/components/Sidebar";
import { SkillsCenter } from "@/pages/SkillsCenter";
import { Packs } from "@/pages/Packs";
import { AgentSkills } from "@/pages/AgentSkills";
import { Models } from "@/pages/Models";
import { AgentModels } from "@/pages/AgentModels";
import { Settings } from "@/pages/Settings";
import { listAgents, type AgentInfo } from "@/lib/api";

export default function App() {
  const [view, setView] = useState<View>("skills");
  const [agents, setAgents] = useState<AgentInfo[]>([]);

  // macOS uses an overlay title bar (traffic lights). Windows/Linux get a
  // native title bar, so no transparent drag strip / top inset there.
  const isMac = useMemo(() => /Mac/i.test(navigator.userAgent), []);
  const topInset = isMac ? 38 : 0;

  // Follow the OS color scheme.
  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => document.documentElement.classList.toggle("dark", mq.matches);
    apply();
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, []);

  useEffect(() => {
    listAgents().then(setAgents).catch(() => setAgents([]));
  }, []);

  return (
    <div className="bg-background text-foreground h-screen overflow-hidden">
      {/* macOS only: transparent draggable strip aligned with the traffic lights */}
      {isMac && (
        <div data-tauri-drag-region className="fixed inset-x-0 top-0 z-50 h-[38px]" />
      )}

      <div className="bg-sidebar grid h-screen grid-cols-[232px_1fr] max-[720px]:grid-cols-[64px_1fr]">
        <Sidebar view={view} onChange={setView} topInset={topInset} />
        <main
          style={{ paddingTop: topInset }}
          className="bg-background border-border flex min-w-0 flex-col overflow-hidden rounded-l-xl border-l"
        >
          {view === "skills" && <SkillsCenter agents={agents} />}
          {view === "packs" && <Packs agents={agents} />}
          {view === "agents" && <AgentSkills agents={agents} />}
          {view === "models" && <Models />}
          {view === "agent-models" && <AgentModels />}
          {view === "settings" && <Settings />}
        </main>
      </div>

      <Toaster position="bottom-right" richColors />
    </div>
  );
}
