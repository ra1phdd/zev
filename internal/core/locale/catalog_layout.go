package locale

import (
	"strings"
)

// catalogLayout keeps renderer-owned whitespace and punctuation out of message catalogs.
type catalogLayout struct {
	prefix string
	suffix string
}

var catalogLayouts = map[Key]catalogLayout{
	Key("cli.no_headers_entered_configuration_unchanged"):         {prefix: "\n"},
	Key("cli.http_headers_updated_successfully"):                  {prefix: "\n"},
	Key("cli.enter_headers_one_per_line_format_header"):           {prefix: "\n", suffix: "\n\n"},
	Key("cli.current_http_headers"):                               {prefix: "\n", suffix: ":"},
	Key("cli.new_headers"):                                        {prefix: "\n", suffix: ":"},
	Key("cli.s_redacted"):                                         {prefix: "  ", suffix: "\n"},
	Key("cli.none_set"):                                           {prefix: "  "},
	Key("cli.history_suggestions"):                                {suffix: ":"},
	Key("cli.history_command_entry"):                              {suffix: "\n"},
	Key("cli.history_command_delivery"):                           {prefix: "     ", suffix: "\n"},
	Key("cli.history_command_warning"):                            {prefix: "     ", suffix: "\n"},
	Key("cli.history_message"):                                    {suffix: "\n"},
	Key("cli.history_role"):                                       {suffix: "\n"},
	Key("cli.current_command_execution_mode_message"):             {suffix: "\n"},
	Key("cli.header_empty_finish"):                                {suffix: ":"},
	Key("cli.status_line"):                                        {suffix: "\n"},
	Key("cli.index_status_line"):                                  {suffix: "\n"},
	Key("cli.embedding_status_line"):                              {suffix: "\n"},
	Key("cli.vectors_rebuilt_ready_d_failed_summary"):             {suffix: "\n"},
	Key("cli.added_s_redacted"):                                   {suffix: "\n"},
	Key("cli.alias_q_added_successfully"):                         {suffix: "\n"},
	Key("cli.alias_q_removed_successfully"):                       {suffix: "\n"},
	Key("cli.alias_q_updated_successfully"):                       {suffix: "\n"},
	Key("cli.command_execution_mode_set_message"):                 {suffix: "\n"},
	Key("cli.saved_command_q_alias_q_run_zev"):                    {suffix: "\n"},
	Key("cli.script_q_created_successfully"):                      {suffix: "\n"},
	Key("cli.script_q_removed_successfully"):                      {suffix: "\n"},
	Key("cli.script_q_updated_successfully"):                      {suffix: "\n"},
	Key("cli.run_recovery_title"):                                 {suffix: ":"},
	Key("cli.run_recovery_analysis_title"):                        {suffix: ":"},
	Key("cli.run_recovery_correction_title"):                      {suffix: ":"},
	Key("cli.run_recovery_warning"):                               {prefix: "   ", suffix: "\n"},
	Key("managed.entity_help"):                                    {suffix: "\n"},
	Key("managed.field_prompt"):                                   {suffix: ": "},
	Key("managed.generation_warning"):                             {suffix: "\n"},
	Key("managed.generation_disclosure"):                          {suffix: "\n"},
	Key("query.no_commands_found_query"):                          {prefix: "\n", suffix: "\n"},
	Key("query.ai_did_not_provide_response_please_try"):           {prefix: "\n", suffix: "\n"},
	Key("query.ai_did_not_provide_response_might_happen"):         {prefix: "\n", suffix: "\n\n"},
	Key("query.unable_process_request_message"):                   {prefix: "\n", suffix: "\n"},
	Key("query.warning_message"):                                  {prefix: "\n", suffix: "\n"},
	Key("query.selected_command_message"):                         {prefix: "\n", suffix: "\n\n"},
	Key("query.attach_context_files_comma_separated_paths_press"): {suffix: ":"},
	Key("query.describe_continue_press_enter_skip"):               {suffix: ":"},
	Key("query.error_message"):                                    {suffix: "\n"},
	Key("query.fetching_suggestions"):                             {suffix: "\n"},
	Key("query.command_failed_fetching_suggestions"):              {suffix: "\n"},
	Key("query.no_commands_found_query_2"):                        {suffix: "\n"},
	Key("query.paste_command_log_enter_newline_ctrl_d"):           {suffix: ":"},
	Key("query.please_clarify_request"):                           {suffix: ":"},
	Key("query.select_command_run"):                               {suffix: ":"},
	Key("query.specify_request"):                                  {suffix: ":"},
	Key("query.request_cannot_empty"):                             {suffix: "\n"},
	Key("query.warning_failed_save_history_message"):              {suffix: "\n"},
	Key("query.zev_version_message"):                              {suffix: "\n"},
	Key("query.warning_s_2"):                                      {suffix: "\n\n"},
	Key("query.usage"):                                            {suffix: ":"},
	Key("query.aliases"):                                          {suffix: ":"},
	Key("query.examples"):                                         {suffix: ":"},
	Key("query.available_commands"):                               {suffix: ":"},
	Key("query.additional_commands"):                              {suffix: ":"},
	Key("query.flags"):                                            {suffix: ":"},
	Key("query.global_flags"):                                     {suffix: ":"},
	Key("query.additional_help_topics"):                           {suffix: ":"},
	Key("ui.setup.current_default"):                               {suffix: ":"},
	Key("ui.setup.current"):                                       {suffix: ":"},
	Key("ui.setup.default"):                                       {suffix: ":"},
	Key("ui.setup.saved"):                                         {prefix: "\n"},
	Key("ui.setup.current_api_key"):                               {prefix: " "},
	Key("update.checking"):                                        {suffix: "\n"},
	Key("update.checking_main"):                                   {suffix: "\n"},
	Key("update.installed"):                                       {suffix: "\n"},
	Key("update.current"):                                         {suffix: "\n"},
	Key("update.migrating"):                                       {suffix: "\n"},
	Key("update.migrations_done"):                                 {suffix: "\n"},
	Key("update.available"):                                       {suffix: "\n"},
	Key("update.available_main"):                                  {suffix: "\n"},
	Key("update.main_warning"):                                    {suffix: "\n"},
	Key("update.main_stable_compatibility_warning"):               {suffix: "\n"},
}

func applyCatalogLayout(key Key, value string) string {
	layout, ok := catalogLayouts[key]
	if !ok {
		return value
	}
	return layout.prefix + value + layout.suffix
}

func normalizeCatalogSource(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ":")
	return strings.TrimSpace(value)
}
