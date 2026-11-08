# Troubleshooting

Fixes for an app that won't open, an agent or provider that fails, a stuck session, and how to report a problem.

## The app won't open

Freeloop releases aren't signed or notarized, so macOS blocks a downloaded copy the first time. Try to open it, then go to System Settings → Privacy & Security and choose **Open Anyway**. An app you built yourself opens normally.

The app needs a Mac with Apple Silicon and macOS 14 or later. After each update the Keychain asks again for access to the keys the app stored. That's expected: the signature changes with every build.

## An agent won't start

Open Settings → Providers. Under **Agents you're signed in to**, each external agent shows **ready** or the reason it can't start.

- "Is not on your login PATH" means the agent's command isn't installed, or your login shell can't find it. Install it, and for Node.js agents install Node.js first.
- An external agent signs in by itself, and Freeloop never sees that sign-in. Run the agent once in a terminal and sign in there, for example `claude` for Claude Code.
- An agent preset you edited by hand refuses to start until you approve it in Settings → Permissions, under **Waiting for your approval**.

If a session's environment won't start, check it too:

- Docker needs Docker running, and a failing **Health check** blocks the session.
- SSH refuses a host key it doesn't know. Run `ssh` to the host once in a terminal and check the fingerprint.
- A sandboxed environment runs no command if the sandbox can't be applied.

## A provider shows an error

In Settings → Providers, choose **Test connection** next to the provider. It shows **Connected** with the number of models, or the error the provider returned.

- Check the key, the address and the model name.
- For a local server such as Ollama or LM Studio, check that it's running.
- If **Local only** is on in Settings → Network, providers elsewhere are unreachable.

## A session is stuck

A session that isn't making progress shows a banner that starts with "Not getting on" and says why: an account limit was reached (and when it resets), the turn limit was reached, the same tool failed three times in a row, there were two rewinds in one turn, or two turns in a row ended in an error.

Use the banner's **Continue on** button, or the **Another profile…** menu, to carry on in a new session in the same project, with another profile such as one on a different provider or agent. The old session's last request and a summary of it go with it.

> Continue on names the profile chosen for that agent under **When a session is stuck** on the profiles page. If an account limit was reached and the session's profile sets `onLimit`, the banner says it continues on that profile when the turn ends, and it does so without asking.

## The sandbox won't start on Linux

On Linux, **Local, sandboxed** runs commands under bubblewrap. Install the `bubblewrap` package if the session says it's missing.

Bubblewrap needs unprivileged user namespaces. Ubuntu 24.04 and later withhold them, through AppArmor, from programs that have no profile of their own, so the session says the sandbox would not start. To give bubblewrap a profile, save this as `/etc/apparmor.d/bwrap`:

```
abi <abi/4.0>,
include <tunables/global>

profile bwrap /usr/bin/bwrap flags=(unconfined) {
  userns,
  include if exists <local/bwrap>
}
```

Then load it with `sudo apparmor_parser -r /etc/apparmor.d/bwrap` and start the session again.

> This lets every program that runs bubblewrap create user namespaces, as Flatpak does; Ubuntu restricts them to make some kernel attacks harder. If you would rather not, use a Docker environment to confine commands instead.

## Configuration problems and logs

If a settings file can't be read, a banner appears: "Configuration problems; using defaults where needed." Each line names the file, line and field. Fix it and save. Freeloop picks up edits within a second, and the banner clears when the problem is gone. Items waiting for your approval aren't problems.

The app's log is `~/Library/Application Support/freeloop/logs/freeloop.log`. It stays on your Mac. In Settings → Data, **Show logs in Finder** opens the folder.

## Report a problem

In Settings → Data, under **Report a problem**, **Copy a report** puts a short diagnostic on your clipboard. **Show what is copied** displays it first.

The report holds Freeloop's version, this Mac's macOS version, model, chip and memory, the agent, provider, model, mode and status of the open session (or the session last open), and the log's last 200 lines. It leaves out your conversation, your files, session titles and your keys. Secrets the app knows are removed, and your home folder's path is written as `~`.

> Log lines can still name your projects, so read the report before you share it.

Nothing is sent anywhere. Paste the report into an issue on the project's GitHub page yourself. For a security problem, use a private security advisory there instead of a public issue.
