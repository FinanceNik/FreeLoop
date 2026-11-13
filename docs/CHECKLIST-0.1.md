# MVP checklist for 0.1.0-beta.1

This is the checklist the M2 definition of done asks for (`docs/ROADMAP.md`): every requirement marked MVP in `docs/REQUIREMENTS.md`, its acceptance criterion in one line, the evidence that checks it and how far that evidence goes. Status is one of four values. **Automated**: a named test or check covers the criterion as written. **Partly**: a test covers some of it, and the cell says what is missing. **Your run**: only you can check it. **Gap**: nothing checks it yet. Rows marked "Your run", and the missing half of "Partly" rows, need your own run on real accounts, real keys, the packaged app or real hardware; they are collected under "For your runs" at the end. Every "Automated" row assumes `make test`, `make lint` and `make check-app` pass on the release commit.

Test names are given without their `Test` prefix where the file is named; Go tests are `go test ./internal/<pkg>/ -run '^TestName$'`.

## Providers

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| PRV-001 Provider registry with built-in types | One provider of each type added from the UI and listed; presets prefill base URL and headers | `internal/engine/capabilities_test.go` `TestPresets` (all presets exist, custom needs a URL); `internal/config/providers_test.go` `TestProvidersRoundTrip`; your run | Partly: adding each type from the form and the prefilled values are not checked |
| PRV-002 Add and test a provider | Wrong key or URL gives a readable error naming the provider; a right one lists models | `internal/provider/anthropic/anthropic_test.go` `TestReadableErrors`; `internal/provider/openaicompat/openaicompat_test.go` `TestListModelsAndErrors`; `internal/engine/capabilities_test.go` `TestOllamaToolSupportGatesSessions` (Test connection lists models) | Partly: the provider's name in the error is in code (`Engine.TestProvider`), not asserted; a real wrong URL not tried |
| PRV-003 Model discovery | Ollama's pulled models appear within 2 s; a manual model ID works without restart | `internal/provider/ollama/ollama_test.go` `TestListAndDescribe`; `frontend/src/models.test.ts` (favorites, hidden) | Partly: the 2 s and a manual model ID in a running app are not checked; real Ollama is your run |
| PRV-004 Streaming with tool calling, normalized | Same transcript, modulo content, on Anthropic, OpenRouter and Ollama for one `read_file` call | `anthropic_test.go` `TestStreamToolUse`; `openaicompat_test.go` `TestStreamOpenAIShape`, `TestStreamToleratesMissingIDsAndIndexReuse`; `ollama_test.go` `TestChatWithToolCallsAndThinking` (recorded stream shapes) | Partly: the three real providers side by side are your run |
| PRV-005 Model capability flags | A model without tool calling is blocked with a reason; an override makes it selectable | `internal/engine/capabilities_test.go` `TestOllamaToolSupportGatesSessions`, `TestLocalModelIsProbedOnceAndCached` | Automated |
| PRV-006 Switch model mid-session | The next turn uses the new model; the transcript records it | `internal/engine/sessions_test.go` `TestSwitchModelMidSession` | Automated |
| PRV-007 Token accounting | Session totals equal the sum of per-turn usage | `frontend/src/transcript.test.ts` "buildView records each model call with its model, per turn"; `internal/engine/usage_test.go` `TestUsageCosts` | Automated |

## Harnesses

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| HRN-001 One harness interface | Conversation, approvals and diff panels work unchanged for built-in and Claude Code | `internal/engine/harnesses_test.go` `TestExternalHarnessSession` (reference agent, approved from the attention view); `scripts/appcheck/scenario.js` "a turn runs to its end, with approvals" (built-in) | Partly: a real Claude Code session in the app is your run |
| HRN-002 Built-in harness | "Add a failing test for X then make it pass" completes with an edit and a shell call shown | `internal/harness/builtin/builtin_test.go` `TestToolLoopEndToEnd`, `TestEditFile` (scripted model) | Partly: the task with a real model on a real repo is your run |
| HRN-003 Declarative harness profiles | Two profiles differing in model and tools run side by side; bad YAML reported with line and field | `internal/engine/profiles_test.go` `TestProfilesRunSideBySide`; `internal/config/profiles_test.go` `TestProfilesLoadAndReportLines` | Automated |
| HRN-004 Choose harness and profile at creation | The new-session form groups profiles by harness, project default preselected | `internal/engine/harnesses_test.go` `TestExternalHarnessSession` (a preset brings its profile) | Partly: the grouping and the preselection in the form are not checked |
| HRN-005 Generic ACP client | A reference ACP agent reads and edits a file, the edit approved in the app | `internal/harness/acp/acp_test.go` `TestReferenceAgent`; `harnesses_test.go` `TestExternalHarnessSession`; `make verify-agents` (`internal/harness/acp/presets_test.go` `TestPresetScenarios`, Gemini CLI 0.66.0 and others on a stand-in model; weekly in CI) | Automated |
| HRN-006 Claude Code harness | A subscription user runs a session; no Anthropic OAuth token in config or database; prompts in the inbox | Your run (`internal/harness/acp/claude_test.go` `TestClaudeCodeHoldsAgainstAHostileRepository` runs the real adapter on a stand-in model, not signed in) | Your run |
| HRN-008 Subscription boundary | No vendor OAuth handling in code; the adapter passes no credentials in env or args; docs state it | `internal/config/boundary_test.go` `TestPresetsCarryNoCredentials` (every built-in preset: no credential on its command line, only known switches in its environment); `internal/permission/review_test.go` `TestVendorSignInsAreNeverRead`; `internal/engine/recap_test.go` `TestRestoreReadsOnlyTheAppsOwnItem`; SECURITY.md, CLAUDE.md | Partly: the search for OAuth handling is by hand; the stored-token check is your run (HRN-006) |
| HRN-009 Plan mode | A write in plan mode is refused and recorded; approving switches to the chosen mode | `internal/harness/builtin/builtin_test.go` `TestPlanModeRefusesWritesAndApprovalSwitchesMode`; app check "a plan reads as a document and is reviewed in the panel" | Automated |
| HRN-010 Instruction files | A rule only in `AGENTS.md` is visibly followed; the files are never modified by the app | `builtin_test.go` `TestNestedAgentsFiles`, `TestInstructionFileLinkedOutsideIsNotRead`; `internal/engine/trust_test.go` `TestProjectTrustAndSettings` (AGENTS.md in the prompt after trust) | Partly: a model following the rule is your run; no test asserts the app never writes the files |

## Skills

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| SKL-001 Agent Skills format is native | An unmodified skill from `~/.claude/skills` loads unchanged | `internal/skill/skill_test.go` `TestDiscoverAndParse` (a Claude Code-shaped skill with extra fields) | Partly: your own `~/.claude/skills` is your run |
| SKL-002 Discovery and toggles | The skills panel lists each skill with source, scope and switch; a disabled one leaves the index | `internal/engine/skills_test.go` `TestSkills` (sources, switches at every scope, index); `internal/skill/skill_test.go` `TestMergePrecedence` | Partly: the Skills page itself is not checked |
| SKL-003 Import from Claude Code | Importing twice does not duplicate; the originals are untouched | `internal/engine/skillinstall_test.go` `TestImportAndInstallSkills` | Automated |
| SKL-004 Progressive disclosure | Under 100 tokens per enabled skill in the prompt; a skill's script prompts unless a rule allows it | `skills_test.go` `TestSkills` (index line under 300 characters, `load_skill` on demand); `internal/permission/broker_test.go` `TestModeTable` (shell asks) | Partly: no test runs a skill's script and sees the prompt |
| SKL-005 Install from folder or zip | Dropping a folder on the skills panel installs it after a preview of frontmatter and scripts | `skillinstall_test.go` `TestImportAndInstallSkills` (zip and folder preview, then install) | Partly: the drop onto the panel is not checked |

## MCP

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| MCP-001 Manage servers | A stdio and a remote HTTP server both show their tools within 5 s of enabling | `internal/mcp/manager_test.go` `TestStdioServer`, `TestHTTPServerGetsHeaders` (local test server) | Partly: the 5 s is not measured; a real remote server is your run |
| MCP-002 Import from Claude Code config | Imported servers work without edits; secrets in env move to the Keychain | `internal/engine/claudeimport_test.go` `TestClaudeImport` | Partly: no test starts an imported server; your run with your `~/.claude.json` |
| MCP-003 MCP tools in the built-in harness | `allow mcp__github__*` approves those tools and nothing else | `internal/engine/mcp_test.go` `TestMCPServersInSessions`; `internal/permission/rules_test.go` `TestRuleMatching` ("tool pattern miss"); `builtin_test.go` `TestMCPToolsAskAndStayOutOfPlanMode` | Automated |
| MCP-004 MCP servers passed to ACP harnesses | A server enabled in a profile is listed by Claude Code in a session with that profile | `acp_test.go` `TestReferenceAgent`; `harnesses_test.go` `TestExternalHarnessSession` (reference agent receives the servers) | Partly: Claude Code listing the server is your run |
| MCP-005 Trust on first start | No server starts without one confirmation per configuration change | `mcp_test.go` `TestMCPServersInSessions`; `manager_test.go` `TestWarning`; `internal/engine/keybinding_test.go` `TestAnMCPApprovalCannotBeWrittenIntoTheFile` | Automated |

## Environments

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| ENV-001 Environment abstraction | No direct `os/exec` or file call outside the environment package, enforced by lint | `internal/archtest/boundary_test.go` `TestBoundaries`, `TestViolationsAreDetected` (`make lint`) | Automated |
| ENV-002 Local environment | A secret on an environment reaches a shell call as a variable and stays out of the transcript | `internal/engine/environments_test.go` `TestEnvironmentSecretsReachCommandsButNotTheTranscript` | Automated |
| ENV-003 Projects and sessions bind to environments | Three sessions on three repos run at once, each with its own cwd and variables | `environments_test.go` `TestThreeSessionsThreeReposConcurrently` | Automated |
| ENV-004 Secret injection | Grepping the config folder and transcripts for a used secret finds nothing | `environments_test.go` `TestEnvironmentSecretsReachCommandsButNotTheTranscript` (walks the data folder); `make test-keychain` for the real Keychain | Automated |

## Interface

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| GUI-001 Main window | Every session state is distinguishable in the sidebar without opening it | `frontend/src/filters.test.ts` "statusSummary counts statuses…"; `frontend/src/sessionActivity.test.ts`; app check "the sidebar names what a waiting session asks" | Partly: only the waiting state is checked in the running app |
| GUI-002 Attention view | Five sessions prompting at once are all resolved from the attention view | `internal/engine/attention_test.go` `TestAttentionListsWaitingAndFailedSessions` (five, resolved from the list); `frontend/src/attentionview.test.ts` | Partly: doing it in the app is your run (M2 definition of done) |
| GUI-003 Conversation rendering | A 5,000-event transcript scrolls at 60 fps on an M1 | OQ-022: measured once in the built-in browser (226 ms open, 1.7 ms per scroll step); app check "a long transcript opens at its end" and the following steps | Your run: 60 fps in the packaged app on an M1 |
| GUI-004 Composer | A message typed during a turn is sent at the next turn boundary; cancel stops within 1 s | `internal/engine/engine_test.go` `TestAQueuedMessageGoesOutWhenTheTurnEnds`, `TestPromptQueueAndCancel` (queued, cancel under 1 s); `frontend/src/commands.test.ts`, `mentions.test.ts` | Partly: cancel against a real provider stream not tried |
| GUI-005 Diff and commit panel | A commit made from the panel appears in `git log` with its message | `internal/engine/changes_test.go` `TestDiffPanel`; `frontend/src/diff.test.ts` | Automated |
| GUI-006 Approval dialog | "Allow always" for `shell: git status` makes a visible rule that approves the next identical call | `internal/engine/capabilities_test.go` `TestAllowAlwaysCreatesVisibleRules`; `frontend/src/shortcuts.test.ts` | Automated |
| GUI-007 Session header controls | Changing permission mode takes effect on the next tool call | `internal/engine/bypass_test.go` `TestTurningBypassOffEndsIt` (a running harness changes mode); `engine_test.go` `TestBypassNeedsOptIn`; app check "the bypass setting" | Partly: no test switches ask to accept-edits mid-session and checks the next call |
| GUI-008 Notifications | Clicking a notification opens the right session | `internal/engine/notices_test.go` `TestNotices` (notices name their session, mutes) | Your run: native delivery needs the packaged app |
| GUI-009 Settings UI | Every MVP setting can be changed without editing files | `frontend/src/settings.test.ts` "every configuration page is listed once"; app check "the app's settings are seven short pages"; `internal/engine/profiles_test.go` `TestProfileYAMLEditor` | Partly: no check walks every MVP setting |
| GUI-010 First-run onboarding | A fresh install reaches a first agent reply in under 5 minutes with an API key | Checked once in server mode with the demo provider (ROADMAP, M2 slice 7); the app check skips the guide | Your run |
| GUI-011 Appearance | Switching system appearance updates the app live | `frontend/src/a11y.test.ts` "contrast follows WCAG's formula" (both themes) | Your run |

## Sessions

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| SES-001 Concurrent sessions | Ten sessions each running a shell command finish without cross-talk in transcripts or cwd | `environments_test.go` `TestThreeSessionsThreeReposConcurrently`; `internal/store/store_test.go` `TestOnAppendOrderUnderConcurrency`; `make memcheck` (ten demo sessions, idle) | Partly: three sessions, not ten running shell commands |
| SES-002 Append-only JSONL transcript | Killing the app mid-stream loses at most the in-flight event | `engine_test.go` `TestAppendedEventIsPersistedThenPublished`; `store_test.go` `TestReopenRecoversSeqAndTruncatesTornTail`; `internal/store/repair_test.go` `TestDamagedTranscriptIsRepaired` | Partly: a torn write is simulated; no test kills a running app |
| SES-003 Restore on restart | After restart a continued built-in prompt has the prior turns in context | `internal/engine/images_test.go` `TestImagesInPrompts` (history reaches the model after reopening); `builtin_test.go` `TestRestoreKeepsWhatFollowsAGap`; `harnesses_test.go` `TestExternalSessionLoad` | Automated |
| SES-004 List, filter, auto-title | A session is findable by its title in one filter step | `frontend/src/filters.test.ts` "a session matches every part of a filter"; `sessions_test.go` `TestAutoTitleRenameArchiveDelete` | Automated |
| SES-005 Archive and delete | A deleted session's files are gone from disk | `sessions_test.go` `TestAutoTitleRenameArchiveDelete`; app check "a session's right-click menu" (deleting asks first) | Automated |
| SES-006 Git state per session | The diff panel still shows the session's changes after an unrelated commit on the branch | `changes_test.go` `TestDiffPanel` (a commit after the base); `environments_test.go` `TestGitStateFollowsTheSession` | Automated |

## Security

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| SEC-001 Permission modes per session | Each mode behaves as specified for a write, a shell call and an MCP call; bypass asks after the setting goes, running or not, and after restart | `internal/permission/broker_test.go` `TestModeTable`; `builtin_test.go` `TestMCPToolsAskAndStayOutOfPlanMode`; `bypass_test.go` `TestTurningBypassOffEndsIt`, `TestBypassDoesNotSurviveARestartWithoutTheSetting`, `TestAutoNeedsAnAgentWithItAndATrustedProject` | Partly: an MCP call is checked in ask and plan only, not in accept-edits or bypass |
| SEC-002 Rule engine | A project deny overrides a global allow; rules are listed with scope and source file | `internal/engine/trust_test.go` `TestProjectTrustAndSettings`; `environments_test.go` `TestEnvironmentScopeRules` (source file); `broker_test.go` `TestRulesInEvaluation`; `internal/engine/restored_settings_test.go` `TestApprovingARestoredRule`; `frontend/src/policy.test.ts` | Automated |
| SEC-003 Single permission broker | An ACP agent's request and a built-in tool's look identical in inbox and transcript | `harnesses_test.go` `TestExternalHarnessSession` (agent requests under the built-in tool names); `internal/harness/acp/approval_test.go` `TestOnlyListedOptionsCanBeChosen` | Partly: no test compares the two side by side |
| SEC-004 Secret storage | No key material in config; an unreadable vault is never overwritten; a pasted secret is redacted | `engine_test.go` `TestProvidersKeepKeysInTheKeychain`; `internal/secrets/vault_test.go` `TestVaultNeverOverwritesWhatItCannotRead`, `TestVaultRemovesNothingUnlessItReadsBack`; `internal/engine/hardening_test.go` `TestStoredKeysAreRedactedFromStartup`; `make test-keychain` | Automated |
| SEC-005 Protected paths | In bypass, writing `~/.ssh/config` is refused and recorded | `broker_test.go` `TestProtectedPathsHoldInBypass`; `builtin_test.go` `TestProtectedPathRefusedInBypass` | Automated |
| SEC-006 Project trust | An untrusted project runs with global config only and says so | `trust_test.go` `TestProjectTrustAndSettings`; `frontend/src/trust.test.ts` | Automated |
| SEC-007 Audit trail | For any file change the transcript shows the call that made it and how it was approved | `broker_test.go` `TestModeTable` (actor per decision); `frontend/src/transcript.test.ts` "panelData lists approvals and finished edits" | Partly: no test follows one file change back to its call and approval |
| SEC-008 Process isolation | `env` inside an external harness shows no app secrets beyond its environment's | `internal/environment/local_test.go` `TestSpawnPipesAndEnvironment`, `TestCommandsGetAMinimalEnvironment` (the path agents and MCP servers are started by) | Automated |
| SEC-009 No undeclared network | With no providers, a network monitor sees no outbound connection in 10 minutes | `internal/engine/updates_test.go` `TestUpdateCheck` (off until turned on) | Your run |
| SEC-010 Untrusted tool output | An instruction planted in a file the agent reads cannot cause an auto-approved write | `internal/engine/untrusted_test.go` `TestAPlantedInstructionApprovesNothing` (the write is still asked about, the mode stays ask, the file's words reach the model only as a tool result); `broker_test.go` `TestModeTable` | Automated (a real model trying is your run) |
| SEC-013 Reads outside the project root | In accept-edits a read of `~/.zshrc` asks; `~/.ssh/id_ed25519` is refused even in bypass | `broker_test.go` `TestReadOutsideRootAsksExceptInBypass`, `TestProtectedPathsHoldInBypass` | Automated |

## Settings

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| SET-001 Human-readable config | Editing a profile file updates the UI within 2 s; a bad file is reported without a crash | `internal/engine/watch_test.go` `TestHotReload`, `TestUnreadableConfigIsKept`; `internal/config/config_test.go` `TestSyntaxErrorIsAnIssueNotACrash` | Partly: the test edits config, providers, rules and environments within 2 s, not a profile |
| SET-002 Project-level overrides | A project profile appears in the new-session form for that project only | `trust_test.go` `TestProjectTrustAndSettings` | Automated |
| SET-003 Import from Claude Code | The preview lists each item with its destination; nothing is written to `~/.claude` | `claudeimport_test.go` `TestClaudeImport`; `skillinstall_test.go` `TestImportAndInstallSkills` | Automated |
| SET-004 Defaults | A new session uses the defaults without extra clicks | `config_test.go` `TestSaveAndFontSize` (defaults saved) | Partly: no test opens the new-session form or creates a session with no choices |

## Extensibility

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| EXT-001 File- and process-based extension points | Every extension type can be added without rebuilding | `harnesses_test.go` `TestExternalHarnessSession` (preset file); `profiles_test.go` `TestProfilesRunSideBySide`; `skills_test.go` `TestSkills`; `mcp_test.go` `TestMCPServersInSessions`; `watch_test.go` `TestHotReload` (providers, environments) | Automated |
| EXT-002 Documented event schema | `schemas/events.json` validates every transcript the test suite produces | `make test` sets `FREELOOP_CHECK_EVENTS`: the store checks every event every Go test writes (`internal/event/validate.go`, `validate_test.go` `TestValidatorCatchesWhatTheSchemaRefuses`); `schema_test.go` | Automated |

## Non-functional

| Requirement | Acceptance | Checked by | Status |
|---|---|---|---|
| NFR-PERF-001 | Input latency under 100 ms with ten sessions streaming, on an M1 with 16 GB | Nothing automated | Your run |
| NFR-PERF-002 | A 5,000-event transcript opens in under 1 s; scrolling virtualized | OQ-022 (226 ms in the built-in browser, once; `content-visibility` instead of a virtual list); app check "a long transcript opens at its end" | Partly: not in the packaged app or on an M1 |
| NFR-PERF-003 | Cold start under 2 s; 50 archived and 10 active sessions restored under 3 s | `internal/engine/restoretime_test.go` `TestRestoringSixtySessionsIsQuick` (60 sessions, 18,000 events: engine, list and newest transcript in about 15 ms on an M-series Mac) | Partly: the window's own start is your run (packaged app) |
| NFR-PERF-005 | Streamed tokens on screen within 50 ms of arriving | `internal/engine/streaming_test.go` `TestStreamedWordsArriveQuickly` (50 words 20 ms apart reach the event bus after about 0.2 ms, 95th percentile under 0.5 ms) | Partly: the window's drawing on top of that is your run |
| NFR-SEC-001 | Secrets only in the Keychain; none in logs, crash reports or exports | `keybinding_test.go` `TestAKeyInAnErrorStaysOutOfTheLog`, `TestRedaction`; `engine_test.go` `TestProvidersKeepKeysInTheKeychain`; `make test-keychain` | Partly: exports and crash traces are not checked for secrets |
| NFR-SEC-002 | Every side-effecting action is attributable in the transcript | As SEC-007 | Partly: as SEC-007 |
| NFR-SEC-003 | Threat model in `docs/SECURITY.md` covers prompt injection, malicious skills and MCP servers, hostile repos, a local attacker | `docs/SECURITY.md` (adversaries A1 to A5 and the scenario table; read for this list) | Partly: met by the document; nothing keeps it current |
| NFR-PRIV-001 | No telemetry, analytics or crash reporting leaves the machine | `updates_test.go` `TestUpdateCheck` (opt-in) | Your run: the network monitor run of SEC-009 |
| NFR-PRIV-002 | Transcripts, config and indexes stay local; export is an explicit action | `internal/engine/export_test.go` `TestExport` (written only where chosen) | Partly: that nothing else is sent is the network monitor run |
| NFR-PRIV-003 | A provider gets only what the transcript shows plus system prompt, tool definitions and skill index, which are inspectable | `internal/engine/modelcontext_test.go` `TestWhatTheModelSeesIsRecorded` (first turn, no change, plan mode, commit message, redaction); `frontend/src/modelContext.test.ts`; `scripts/appcheck/scenario.js` "the Info tab says what the model sees" | Automated for the built-in agent; an external agent builds its own requests and says so |
| NFR-OFF-001 | With a local model and local MCP servers, every MVP feature works offline | `internal/engine/localonly_test.go` `TestLocalOnly`; `internal/netguard/netguard_test.go` `TestLocalOnly` | Partly: a session with networking off is your run |
| NFR-OFF-002 | No startup network dependency; nothing degrades when an update check fails | `updates_test.go` `TestUpdateCheckFailures` | Partly: a launch with networking off is your run |
| NFR-XPLAT-001 | macOS 14 and newer, Apple Silicon only | `make package` fails unless the binary is `arm64` alone (`build/darwin/Taskfile.yml`); `LSArchitecturePriority` arm64 and `LSMinimumSystemVersion` 14.0.0 in `build/darwin/Info.plist`; CI on `macos-15` | Partly: nothing runs on macOS 14 itself |
| NFR-XPLAT-002 | No system-specific code outside a platform package and the environment layer's per-system files | `internal/archtest/boundary_test.go` `TestBoundaries` (macOS-only files and build tags, cgo, the system-call packages, imports of the platform packages) | Partly: the test does not look for `_linux.go` or `_windows.go` files or their build tags outside those places |
| NFR-A11Y-001 | Full keyboard use of conversation, approvals and session switching; visible focus | `frontend/src/a11y.test.ts` "arrow keys move between tabs and wrap"; `shortcuts.test.ts`; `attentionview.test.ts`; app check "panes fit, and the keyboard moves between them" | Partly: the full keyboard walk was by hand on 2026-11-01; your run |
| NFR-A11Y-002 | Labels, announced approvals, WCAG AA contrast, reduced motion, text follows Font size | `a11y.test.ts` (contrast, no fixed pixel sizes, announced picker entry); app check "every control has a name" | Partly: announcements and reduced motion are not checked; VoiceOver is your run |
| NFR-REL-001 | A GUI crash never loses received events; sessions recover on relaunch | `engine_test.go` `TestCancelAndRecovery` (simulated crash); `store_test.go` `TestReopenRecoversSeqAndTruncatesTornTail`; `internal/store/recover_test.go` `TestRebuildsAMissingDatabase`; `leftovers_test.go` `TestOpenStopsWhatACrashLeftRunning` (NFR-REL-005) | Partly: no test kills the running app |
| NFR-REL-002 | A crashed external harness or MCP server stays in its session as an error; the next message restarts it | `internal/engine/resilience_test.go` `TestAnExternalAgentIsStartedAgainAfterItExits`, `TestAPanicEndsOnlyItsTurn` | Partly: an MCP server crashing mid-session is not tested |
| NFR-REL-003 | Engine unit tests; replayable provider and ACP fixtures; CI runs the tests on every commit on Linux, and the macOS build, lint and real-app check on tags, weekly and by hand | `.github/workflows/ci.yml` (the `test` job on Linux and the `windows` job on every push and pull request; the `build` job on `macos-15` runs `make test`, lint and `make check-app` on tags and by hand); recorded stream shapes in `internal/provider/*`; the reference agent in `internal/harness/acp/acptest` | Partly: the weekly run skips the build, lint and app check and runs only the real agents |

## For your runs

1. **Providers from the form.** In the packaged app, add an Anthropic, an OpenAI-compatible (OpenRouter preset) and an Ollama provider; check the preset fills the URL; try a wrong key and a wrong URL and read the error; add a model by ID and use it without restarting. Covers PRV-001, PRV-002, PRV-003. *2026-11-11: the user added a custom provider from the form with an API key and a base URL, and ran it. Not yet recorded: Anthropic and Ollama from the form, a preset filling the URL, a wrong key and a wrong URL, a model added by ID.*
2. **The M1 real-provider run.** Three sessions on three repos at once, one each on Anthropic, OpenRouter and Ollama, each doing "add a failing test for X, then make it pass" with edits approved in the app; include a rule stated only in `AGENTS.md`, and plant an instruction in a file the agent reads. Quit, relaunch, continue each in context. Then grep the data folder and transcripts for the API key. Covers PRV-004, HRN-002, HRN-010, SEC-010, SES-003, ENV-004.
3. **Claude Code on your subscription.** Sign in with `claude` in a terminal, run a session from the app with a profile that enables an MCP server, approve its prompts from the inbox, and check the Diff and Approvals tabs. Ask it to run `env` and look for app secrets. Afterwards grep the data folder and `freeloop.db` for an OAuth token (`sk-ant-oat`). Covers HRN-006, HRN-008, HRN-001, MCP-004, SEC-003, SEC-008, and the M2 item "a subscription session with no token stored". *2026-11-07: the user ran a Claude Code session on the subscription from the app and reported it worked as expected ("looks great"). 2026-11-08: a full day of work ran as Claude Code on the subscription inside the app, its approvals answered there. Checked from inside that session: its environment holds no provider key and nothing of the app's (only Claude Code's own variables and `SSH_AUTH_SOCK`, which every command gets so git over SSH works); the data folder and `freeloop.db` hold no `sk-ant-oat` token, only the word in transcripts, and the one key-shaped string found is the fake key of `internal/engine/security_test.go`, read by an agent. Not yet recorded: a profile enabling an MCP server.*
4. **Imports in one built-in session.** Import a skill from `~/.claude/skills` and an MCP server from `~/.claude.json`, approve the server, and use both in one built-in session. Also enable a remote Streamable HTTP server and time its tools appearing (5 s). Covers SKL-001, SKL-003, MCP-001, MCP-002, SET-003, and the M2 item on imports.
5. **Five approvals from the attention view.** Get five sessions waiting at once and answer all of them from the attention view alone. Covers GUI-002 and the M2 item.
6. **Ten busy sessions on an M1 with 16 GB.** Ten sessions streaming and running shell commands at once; type in the composer and judge the lag; check no transcript or cwd crosses over. Covers SES-001, NFR-PERF-001, NFR-PERF-005 (by eye, until it is measured).
7. **Long transcript and launch times.** In the packaged app on an M1, open a 5,000-event session (time to open, smoothness of scrolling); time a cold start, and a start with 50 archived and 10 active sessions. Covers GUI-003, NFR-PERF-002, NFR-PERF-003.
8. **Notifications.** With the window in the background: allow notifications when macOS asks, get an approval request, a failure and a finished turn; click a notification; check the dock badge and a muted project. Covers GUI-008.
9. **First run with a key at hand.** On an empty `FREELOOP_HOME`, go from launch to a first agent reply with a real key, timed (under 5 minutes). Covers GUI-010.
10. **Appearance.** Switch macOS between light and dark while the app is open. Covers GUI-011.
11. **Network monitor.** With no providers configured, watch the app for 10 minutes in a network monitor (LuLu, Little Snitch or `nettop -p`). Covers SEC-009, NFR-PRIV-001, NFR-PRIV-002.
12. **Offline.** Turn networking off; launch the app; run a session on Ollama with a local stdio MCP server. Covers NFR-OFF-001, NFR-OFF-002.
13. **Keyboard and VoiceOver.** Operate conversation, approvals and session switching by keyboard only; a short VoiceOver pass over an approval; turn on Reduce Motion. Covers NFR-A11Y-001, NFR-A11Y-002.
14. **Keychain on the release build.** `make test-keychain`; then install the release build over the previous one and check it asks for the Keychain once. Covers SEC-004, ENV-004, NFR-SEC-001.
15. **A hard kill.** `kill -9` the packaged app while an agent is streaming; relaunch and check the transcript. Covers SES-002, NFR-REL-001.
16. **Settings and forms walk.** Change every MVP setting from the UI without touching files; drop a skill folder onto the Skills page; turn a skill off and check the Skills page shows source and scope; check the new-session form groups profiles by harness with the project default preselected and applies the defaults; switch ask to accept-edits mid-turn and watch the next edit; edit a profile file by hand and watch the UI. Covers GUI-009, SKL-002, SKL-005, HRN-004, SET-004, GUI-001, GUI-007, SET-001.
17. **Supported systems.** Run the DMG on macOS 14 on Apple Silicon, if one is at hand (Intel was dropped on 2026-11-08). Covers NFR-XPLAT-001.
18. **Two weeks of daily use** with no transcript loss and no session cross-talk (the M2 item). Covers SES-001, SES-002, NFR-REL-001, NFR-REL-002.
