# Providers

This page explains how to connect the built-in agent to a model, and how to choose and change the model for a session.

## What a provider is

A provider is the place the built-in agent sends its model calls: an API key for a hosted service, or the address of a model that runs on your Mac. You add providers in **Settings → Providers**.

External agents such as Claude Code, Codex CLI, Gemini CLI, OpenCode and goose do not use providers. Each one brings its own model access and sign-in, and Freeloop passes it no credentials. They appear on the same page under **Agents you're signed in to**, with a button to start a session with them.

## Kinds

When you click **Add provider**, the **Kind** list offers these:

| Kind | Notes |
| --- | --- |
| Anthropic | Needs an API key. |
| OpenAI (Responses API) | OpenAI's native API. It keeps reasoning summaries and reasoning across tool calls. |
| Google Gemini | Needs an API key from Google AI Studio. |
| Ollama (local) | Pull a model with tool support first, for example `ollama pull qwen3`. |
| OpenAI | OpenAI through the Chat Completions API. |
| OpenRouter | Not every model on it supports tools. Pick one that does. |
| LM Studio (local) | A local server. |
| llama.cpp server (local) | Start `llama-server` with `--jinja`, or tool calls do not work. |
| vLLM (local) | Start it with `--enable-auto-tool-choice` and a `--tool-call-parser`. |
| Ollama (OpenAI-compatible) (local) | The native Ollama kind is usually the better choice. |
| Custom OpenAI-compatible (local) | Any server that speaks the OpenAI API. You enter the **Base URL**. |

Choosing a kind fills in the **Base URL**, and for Anthropic, OpenAI (Responses API) and Google Gemini also a default model.

## Add a provider

1. Open **Settings → Providers** and click **Add provider**.
2. Choose the **Kind** and give it a **Name**.
3. Enter the **API key**. It is optional for local servers.
4. Optionally set a **Default model**.
5. Click **Save**.

Freeloop tests the connection as soon as you save. You can run the test again at any time with **Test connection**. A working provider shows **Connected** and the number of models the endpoint lists. A failed test shows the error.

## Keys and addresses

API keys are stored in the macOS Keychain. Freeloop never shows a saved key again. When you edit a provider, leave the key empty to keep the saved one. **Delete** removes the provider and its key from the Keychain.

Freeloop connects only to the endpoints you add. If a provider's address is changed outside the app, the key is not sent and the provider shows a warning. Click **Use this address** only if you made that change.

## Models

The new-session form has a **Model** field. It lists the models the endpoint reports, and you can also type any model ID. This is how you use a model the endpoint does not list.

Under the field you can mark the chosen model:

* **☆ Favorite** puts it first in the list, whether or not the endpoint lists it. The button then reads **★ Unfavorite**.
* **Hide** leaves it out of the list. Tick **Show hidden** to see hidden models again, and use **Unhide** to restore one.

## Tool calling

The built-in agent works through tools, so a model must be able to call them. Under the model, Freeloop shows what it knows:

* **Tool calling supported** with a source: known for this provider, reported by the server, tested, or set by you.
* **Tool support is not known yet.** Click **Test now**. Freeloop asks the model to call a trivial tool and checks that the call comes back as a real tool call. Local models can take a minute to load. If you do not test, it is tested when the session starts.
* **This model cannot be used by the built-in harness.** The model answered in text, or the server reports no tool support, so **Start session** is disabled.

If you know better, click **Use anyway** to override the result. **Undo override** removes it. A model that cannot really call tools will fail during the session.

## Switch model during a session

For a built-in session, open the command palette with ⌘K and choose **Switch model…**. A bar appears with **Next turn uses**, a provider list and a model field. Click **Switch**. The next model call uses the new model with the whole conversation, and the transcript records the change. If a turn is running, its next model call already uses the new model. The new model must be able to call tools.

External agents choose their own model.
