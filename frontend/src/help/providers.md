# Providers

Connect the built-in agent to a model, then choose and change the model for a session.

## Add a provider

A provider is where the built-in agent sends its model calls: an API key for a hosted service, or the address of a model on your computer.

- Open **Settings → Providers** and click **Add provider**.
- Choose the Kind and give it a Name.
- Enter the API key. It's optional for local servers.
- Optionally set a Default model, then click **Save**.

Freeloop tests the connection when you save. A working provider shows **Connected** and the number of models listed, and a failure shows the error. Run it again with **Test connection**.

External agents such as Claude Code don't use providers. They bring their own sign-in and appear on the same page under Agents you're signed in to.

## Pick a kind

Choosing a kind fills in the Base URL, and for Anthropic, OpenAI (Responses API) and Google Gemini also a default model.

| Kind | Notes |
| --- | --- |
| Anthropic | Needs an API key. |
| OpenAI (Responses API) | OpenAI's native API. It keeps reasoning summaries and reasoning across tool calls. |
| Google Gemini | Needs an API key from Google AI Studio. |
| OpenAI | OpenAI through the Chat Completions API. |
| OpenRouter | Not every model on it supports tools. Pick one that does. |
| Ollama (local) | Pull a model with tool support first, for example `ollama pull qwen3`. |
| LM Studio (local) | A local server. |
| llama.cpp server (local) | Start `llama-server` with `--jinja`, or tool calls don't work. |
| vLLM (local) | Start it with `--enable-auto-tool-choice` and a `--tool-call-parser`. |
| Ollama (OpenAI-compatible) (local) | The native Ollama kind is usually the better choice. |
| Custom OpenAI-compatible (local) | Any server that speaks the OpenAI API. You enter the Base URL. |

## Keys and addresses

API keys are stored in {{the macOS Keychain|Windows Credential Manager|the system keyring}}, and Freeloop never shows a saved key again. When you edit a provider, leave the key empty to keep it. **Delete** removes the provider and its key.

> Freeloop connects only to the endpoints you add. If a provider's address is changed outside the app, the key isn't sent and the provider shows a warning. Click **Use this address** only if you made that change.

## Choose a model

The new-session form's **Model** field lists the models the endpoint reports. You can also type any model ID, to use a model the endpoint doesn't list.

- **☆ Favorite** puts the chosen model first in the list.
- **Hide** leaves it out. Tick **Show hidden** to see hidden models, and use Unhide to restore one.

## Check tool calling

The built-in agent works through tools, so a model must be able to call them. Under the model, Freeloop shows what it knows:

- Tool calling supported: known for this provider, reported by the server, tested, or set by you.
- Tool support is not known yet: click **Test now**, which asks the model to call a trivial tool. Local models can take a minute to load. If you don't test, it's tested when the session starts.
- This model cannot be used by the built-in harness: the model answered in text or the server reports no tool support, so Start session is disabled.

**Use anyway** overrides the result, and Undo override removes it.

> A model that can't really call tools will fail during the session.

## Switch model during a session

For a built-in session, press ⌘K and choose **Switch model…**. Pick a provider and a model under **Next turn uses**, then click **Switch**.

The next model call uses the new model with the whole conversation, and the transcript records the change. The new model must be able to call tools. External agents choose their own model.
