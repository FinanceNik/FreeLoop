package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/rpc"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// The command line (EXT-004) is a client of the running app's headless
// API: what it starts shows in the app at once, and approvals stay with
// the app's permission broker.

var commands = []string{"sessions", "new", "prompt", "events", "approvals", "approve", "deny", "optimizer", "help"}

func isCommand(arg string) bool { return slices.Contains(commands, arg) }

const usage = `freeloop <command> [arguments]

  sessions                       list sessions
  new <folder> [-title t] [-profile p] [-provider id] [-model m] [-mode m] [-environment e] [-optimizer o]
                                 start a session in a project folder; prints its ID
  prompt <session> <text…>       send a message and print the reply as it comes
  events <session> [-after n]    print a session's events as JSON lines
  approvals                      list requests waiting for a decision
  approve <request> [option]     allow a request (allow_once unless an option is given)
  deny <request>                 deny a request
  optimizer <session> [id|off]   list a session's token optimizers, or switch to one

The app must be running; the command line talks to it.
`

func runCLI(sock string, args []string, out, errOut io.Writer) int {
	if args[0] == "help" {
		fmt.Fprint(out, usage)
		return 0
	}
	c, err := rpc.Dial(sock)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	defer c.Close()
	if err := command(c, args, plainWriter{out}); err != nil {
		fmt.Fprintln(plainWriter{errOut}, err)
		return 1
	}
	return 0
}

// plainWriter drops control characters from what goes to the terminal.
// Titles and replies come from the model, and a shell call's title is its
// command: escape sequences in one could hide the real command at an approval.
type plainWriter struct{ w io.Writer }

func (p plainWriter) Write(b []byte) (int, error) {
	_, err := io.WriteString(p.w, plainText(string(b)))
	return len(b), err
}

func plainText(s string) string {
	return strings.Map(func(r rune) rune {
		// Format characters too: bidi overrides reorder what is shown, and
		// zero-width ones hide what a line says.
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
			return -1
		}
		return r
	}, s)
}

// oneLine keeps a title on its line, so one cannot fake the rows after it.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r\n", " ⏎ ", "\n", " ⏎ ", "\r", " ⏎ ").Replace(s)), " ")
}

func marker(on bool) string {
	if on {
		return "*"
	}
	return " "
}

func command(c *rpc.Client, args []string, out io.Writer) error {
	switch args[0] {
	case "sessions":
		var list []session.Session
		if err := c.Call("sessions.list", nil, &list); err != nil {
			return err
		}
		for _, s := range list {
			if s.ParentID != "" {
				continue
			}
			fmt.Fprintf(out, "%s  %-18s %s  (%s)\n", s.ID, s.Status, oneLine(s.Title), filepath.Base(s.ProjectRoot))
		}
		return nil
	case "new":
		fs := flag.NewFlagSet("new", flag.ContinueOnError)
		title := fs.String("title", "", "the session's title")
		profile := fs.String("profile", "", "the profile to start with")
		mode := fs.String("mode", "", "ask, accept_edits or plan")
		prov := fs.String("provider", "", "the provider's ID (the Settings default when empty)")
		model := fs.String("model", "", "the model")
		environmentID := fs.String("environment", "", "the environment's ID, such as sandboxed")
		optimizer := fs.String("optimizer", "", "a token optimizer's ID, or off (the profile's, then the Settings default when empty)")
		if len(args) < 2 {
			return fmt.Errorf("usage: freeloop new <folder> [-title t] [-profile p] [-mode m]")
		}
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		root, err := filepath.Abs(args[1])
		if err != nil {
			return err
		}
		var s session.Session
		if err := c.Call("sessions.create", engine.CreateSessionRequest{Title: *title, ProjectRoot: root, ProfileID: *profile, Mode: *mode, ProviderID: *prov, Model: *model, EnvironmentID: *environmentID, Optimizer: *optimizer}, &s); err != nil {
			return err
		}
		fmt.Fprintln(out, s.ID)
		return nil
	case "optimizer":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: freeloop optimizer <session> [id|off]")
		}
		if len(args) == 3 {
			return c.Call("sessions.setOptimizer", map[string]any{"id": args[1], "optimizer": args[2]}, nil)
		}
		var ses session.Session
		if err := c.Call("sessions.get", map[string]any{"id": args[1]}, &ses); err != nil {
			return err
		}
		var list []engine.OptimizerInfo
		if err := c.Call("sessions.optimizers", map[string]any{"id": args[1]}, &list); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s off\n", marker(ses.Optimizer == ""))
		for _, o := range list {
			fmt.Fprintf(out, "%s %s  %s", marker(ses.Optimizer == o.ID), o.ID, oneLine(o.Name))
			if o.Reason != "" {
				fmt.Fprintf(out, " (%s)", oneLine(o.Reason))
			}
			fmt.Fprintln(out)
		}
		return nil
	case "prompt":
		if len(args) < 3 {
			return fmt.Errorf("usage: freeloop prompt <session> <text…>")
		}
		return prompt(c, args[1], strings.Join(args[2:], " "), out)
	case "events":
		fs := flag.NewFlagSet("events", flag.ContinueOnError)
		after := fs.Int64("after", 0, "only events after this seq")
		if len(args) < 2 {
			return fmt.Errorf("usage: freeloop events <session> [-after n]")
		}
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		var evs []json.RawMessage
		if err := c.Call("sessions.events", map[string]any{"id": args[1], "after": *after}, &evs); err != nil {
			return err
		}
		for _, ev := range evs {
			fmt.Fprintln(out, string(ev))
		}
		return nil
	case "approvals":
		var pending []permission.Request
		if err := c.Call("approvals.list", nil, &pending); err != nil {
			return err
		}
		for _, r := range pending {
			var opts []string
			for _, o := range r.Options {
				opts = append(opts, o.ID)
			}
			fmt.Fprintf(out, "%s  %s  %s  [%s]\n", r.ID, r.SessionID, oneLine(r.Title), strings.Join(opts, " "))
		}
		return nil
	case "approve", "deny":
		if len(args) < 2 {
			return fmt.Errorf("usage: freeloop %s <request>", args[0])
		}
		option := "allow_once"
		if args[0] == "deny" {
			option = "reject_once"
		} else if len(args) > 2 {
			option = args[2]
		}
		return c.Call("approvals.decide", map[string]any{"requestId": args[1], "optionId": option}, nil)
	}
	return fmt.Errorf("unknown command %q; see freeloop help", args[0])
}

// prompt sends a message and prints the agent's reply as it streams, and
// tool calls and requests waiting for the user as they come, until the
// turn ends.
func prompt(c *rpc.Client, sid, text string, out io.Writer) error {
	var before []event.Event
	if err := c.Call("sessions.events", map[string]any{"id": sid, "after": 0}, &before); err != nil {
		return err
	}
	var after int64
	if len(before) > 0 {
		after = before[len(before)-1].Seq
	}
	if err := c.Call("sessions.prompt", map[string]any{"id": sid, "text": text}, nil); err != nil {
		return err
	}
	for deadline := time.Now().Add(time.Hour); time.Now().Before(deadline); {
		var evs []event.Event
		if err := c.Call("sessions.wait", map[string]any{"id": sid, "after": after, "timeoutMs": 30000}, &evs); err != nil {
			return err
		}
		for _, ev := range evs {
			after = ev.Seq
			var d map[string]any
			json.Unmarshal(ev.Data, &d)
			switch ev.Type {
			case event.AgentMessageChunk:
				var m event.MessageChunk
				json.Unmarshal(ev.Data, &m)
				fmt.Fprint(out, m.Content.Text)
			case event.ToolCall:
				fmt.Fprintf(out, "\n[%s]\n", oneLine(fmt.Sprint(d["title"])))
			case event.PermissionRequested:
				fmt.Fprintf(out, "\n[waiting for approval: %s; freeloop approve %v]\n", oneLine(fmt.Sprint(d["title"])), d["requestId"])
			case event.Error:
				fmt.Fprintf(out, "\n[error: %v]\n", d["message"])
			case event.TurnEnded:
				fmt.Fprintln(out)
				if r, _ := d["stopReason"].(string); r != "" && r != "end_turn" {
					return fmt.Errorf("the turn ended: %s", r)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("gave up waiting for the turn to end")
}
