import { invoke } from "@tauri-apps/api/core";

export type AgentInfo = { id: string; name: string; color: string };

export type Skill = {
  name: string;
  description: string;
  assigned_agents: string[];
};

export type Pack = {
  name: string;
  description: string;
  skills: string[];
  assigned_agents: string[];
};

export type AgentSkillSource = "center" | "real" | "external";
export type AgentSkill = {
  name: string;
  description: string;
  source: AgentSkillSource;
  target: string;
};

export type AgentDirInfo = {
  id: string;
  name: string;
  color: string;
  path: string;
  exists: boolean;
  skill_count: number;
  removable: boolean;
};

export type CandidateAgent = {
  id: string;
  name: string;
  color: string;
  rel_dir: string;
};

export type LlmConfig = {
  endpoint: string;
  model: string;
  has_key: boolean;
};

export type ModelInfo = {
  id: string;
  name: string;
  context_window: number | null;
  max_output: number | null;
  capabilities: string[];
};

export type Provider = {
  key: string;
  name: string;
  endpoint_openai: string;
  endpoint_claude: string;
  site: string;
  color: string;
  initials: string;
  has_key: boolean;
  models: ModelInfo[];
};

export type ClaudeAgentModelOptions = {
  same_model_for_slots: boolean;
  haiku_model: string;
  sonnet_model: string;
  opus_model: string;
};

export type KimiAgentModelOptions = {
  provider_type: string;
  capabilities: string[];
};

export type OpencodeAgentModelOptions = {
  provider_type: string;
};

export type CodexAgentModelOptions = {
  env_key: string;
};

export type AgentModelOptions = {
  claude: ClaudeAgentModelOptions;
  kimi: KimiAgentModelOptions;
  opencode: OpencodeAgentModelOptions;
  codex: CodexAgentModelOptions;
};

export type AgentModelFile = {
  path: string;
  content: string;
};

export type AgentModelConfig = {
  agent: AgentInfo;
  config_path: string;
  secondary_config_path: string;
  endpoint_kind: "claude" | "openai";
  configured: boolean;
  provider: string;
  model: string;
  options: AgentModelOptions;
  resolved_provider: Provider | null;
  resolved_model: ModelInfo | null;
  error: string;
};

export type AgentModelSyncResult = {
  agent: string;
  ok: boolean;
  message: string;
};

export type Translation = {
  text: string;
  cached: boolean;
};

export type DetectedSkill = {
  rel_path: string;
  name: string;
  description: string;
  exists: boolean;
};

export type ParsedImport = {
  root: string;
  is_temp: boolean;
  skills: DetectedSkill[];
};

export type ImportSelection = {
  rel_path: string;
  name: string;
};

export type ImportResult = {
  imported: string[];
  errors: string[];
};

// ─── agents ───
export const listAgents = () => invoke<AgentInfo[]>("list_agents");
export const listAgentDirs = () => invoke<AgentDirInfo[]>("list_agent_dirs");
export const createAgentDir = (agent: string) =>
  invoke<void>("create_agent_dir", { agent });
export const listCandidateAgents = () =>
  invoke<CandidateAgent[]>("list_candidate_agents");
export const addAgent = (id: string) =>
  invoke<AgentDirInfo>("add_agent", { id });
export const removeAgent = (id: string) =>
  invoke<void>("remove_agent", { id });

// ─── skill center ───
export const listSkills = () => invoke<Skill[]>("list_skills");
export const readSkillMd = (name: string) =>
  invoke<string>("read_skill_md", { name });
export const createSkill = (name: string, description: string) =>
  invoke<void>("create_skill", { name, description });
export const deleteSkill = (name: string) =>
  invoke<void>("delete_skill", { name });
export const assignSkill = (skill: string, agent: string) =>
  invoke<void>("assign_skill", { skill, agent });
export const unassignSkill = (skill: string, agent: string) =>
  invoke<void>("unassign_skill", { skill, agent });

// ─── packs ───
export const listPacks = () => invoke<Pack[]>("list_packs");
export const createPack = (name: string, description: string) =>
  invoke<void>("create_pack", { name, description });
export const deletePack = (name: string) =>
  invoke<void>("delete_pack", { name });
export const renamePack = (name: string, newName: string, description: string) =>
  invoke<void>("rename_pack", { name, newName, description });
export const addSkillToPack = (pack: string, skill: string) =>
  invoke<void>("add_skill_to_pack", { pack, skill });
export const removeSkillFromPack = (pack: string, skill: string) =>
  invoke<void>("remove_skill_from_pack", { pack, skill });
export const assignPack = (pack: string, agent: string) =>
  invoke<void>("assign_pack", { pack, agent });
export const unassignPack = (pack: string, agent: string) =>
  invoke<void>("unassign_pack", { pack, agent });

// ─── agent skills ───
export const listAgentSkills = (agent: string) =>
  invoke<AgentSkill[]>("list_agent_skills", { agent });
export const removeAgentSkill = (agent: string, name: string) =>
  invoke<void>("remove_agent_skill", { agent, name });
export const importSkill = (agent: string, name: string) =>
  invoke<void>("import_skill", { agent, name });

// ─── misc ───
export const openPath = (path: string) => invoke<void>("open_path", { path });

// ─── llm config ───
export const getLlmConfig = () => invoke<LlmConfig>("get_llm_config");
export const setLlmConfig = (
  endpoint: string,
  model: string,
  apiKey: string | null,
) => invoke<void>("set_llm_config", { endpoint, model, apiKey });

// ─── agent model config ───
export const listAgentModelConfigs = () =>
  invoke<AgentModelConfig[]>("list_agent_model_configs");
export const previewAgentModelConfig = (
  agent: string,
  provider: string,
  model: string,
  options: AgentModelOptions,
) =>
  invoke<AgentModelFile[]>("preview_agent_model_config", {
    agent,
    provider,
    model,
    options,
  });
export const setAgentModelConfig = (
  agent: string,
  provider: string,
  model: string,
  options: AgentModelOptions,
) =>
  invoke<AgentModelConfig>("set_agent_model_config", {
    agent,
    provider,
    model,
    options,
  });
export const syncAgentModelConfig = (agent: string) =>
  invoke<AgentModelConfig>("sync_agent_model_config", { agent });
export const syncAllAgentModelConfigs = () =>
  invoke<AgentModelSyncResult[]>("sync_all_agent_model_configs");

// ─── model center ───
export const listProviders = () => invoke<Provider[]>("list_providers");

export const createProvider = (
  key: string,
  name: string,
  endpointOpenai: string,
  endpointClaude: string,
  site: string,
  apiKey: string | null,
) =>
  invoke<Provider>("create_provider", {
    key,
    name,
    endpointOpenai,
    endpointClaude,
    site,
    apiKey,
  });
export const updateProvider = (
  key: string,
  newKey: string,
  name: string,
  endpointOpenai: string,
  endpointClaude: string,
  site: string,
  apiKey: string | null,
) =>
  invoke<Provider>("update_provider", {
    key,
    newKey,
    name,
    endpointOpenai,
    endpointClaude,
    site,
    apiKey,
  });
export const deleteProvider = (key: string) =>
  invoke<void>("delete_provider", { key });
export const addModel = (
  provider: string,
  id: string,
  name: string,
  contextWindow: number | null,
  maxOutput: number | null,
  capabilities: string[],
) =>
  invoke<Provider>("add_model", {
    provider,
    id,
    name,
    contextWindow,
    maxOutput,
    capabilities,
  });
export const updateModel = (
  provider: string,
  id: string,
  name: string,
  contextWindow: number | null,
  maxOutput: number | null,
  capabilities: string[],
) =>
  invoke<Provider>("update_model", {
    provider,
    id,
    name,
    contextWindow,
    maxOutput,
    capabilities,
  });
export const removeModel = (provider: string, id: string) =>
  invoke<Provider>("remove_model", { provider, id });

// ─── translation ───
export const getSkillTranslation = (name: string) =>
  invoke<string | null>("get_skill_translation", { name });
export const translateSkill = (name: string) =>
  invoke<Translation>("translate_skill", { name });

// ─── import ───
export const parseGithubImport = (link: string, branch: string | null) =>
  invoke<ParsedImport>("parse_github_import", { link, branch });
export const parseUrlImport = (url: string) =>
  invoke<ParsedImport>("parse_url_import", { url });
export const parseLocalImport = (path: string) =>
  invoke<ParsedImport>("parse_local_import", { path });
export const importFromStaging = (
  root: string,
  isTemp: boolean,
  selections: ImportSelection[],
) => invoke<ImportResult>("import_from_staging", { root, isTemp, selections });
export const cancelImport = (root: string, isTemp: boolean) =>
  invoke<void>("cancel_import", { root, isTemp });
export const createSkillText = (name: string, content: string) =>
  invoke<void>("create_skill_text", { name, content });
