mod commands;
mod config;
mod import;
mod meta;
mod packs;
mod paths;

use commands::*;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .invoke_handler(tauri::generate_handler![
            list_agents,
            list_agent_dirs,
            create_agent_dir,
            list_skills,
            read_skill_md,
            create_skill,
            delete_skill,
            assign_skill,
            unassign_skill,
            list_packs,
            create_pack,
            delete_pack,
            rename_pack,
            add_skill_to_pack,
            remove_skill_from_pack,
            assign_pack,
            unassign_pack,
            list_agent_skills,
            remove_agent_skill,
            import_skill,
            open_path,
            get_llm_config,
            set_llm_config,
            list_agent_model_configs,
            preview_agent_model_config,
            set_agent_model_config,
            sync_agent_model_config,
            sync_all_agent_model_configs,
            list_providers,
            create_provider,
            update_provider,
            delete_provider,
            add_model,
            update_model,
            remove_model,
            get_skill_translation,
            translate_skill,
            parse_github_import,
            parse_url_import,
            parse_local_import,
            import_from_staging,
            cancel_import,
            create_skill_text,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
