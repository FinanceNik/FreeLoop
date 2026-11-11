package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"sync/atomic"
	"time"

	"github.com/FinanceNik/Freeloop/frontend"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/remote"
)

// phone runs the phone server (GUI-052) and keeps the app going for it while the
// user is away: no App Nap while it listens, and, if asked, no idle sleep while a
// session works or waits.
type phone struct {
	srv   *remote.Server
	eng   *engine.Engine
	awake atomic.Bool
}

func startPhone(ctx context.Context, eng *engine.Engine, dataDir string) *phone {
	// A build made without the phone app serves an empty folder.
	var files fs.FS = embed.FS{}
	if sub, err := fs.Sub(frontend.Assets(), "phone"); err == nil {
		files = sub
	} else {
		log.Printf("freeloop: phone app: %v", err)
	}
	p := &phone{srv: remote.New(eng, dataDir, files), eng: eng}
	go p.srv.Run(ctx)
	go p.stayAwake(ctx)
	return p
}

// on says the phone server is listening, so the app must not quit with its window.
func (p *phone) on() bool { return p != nil && p.srv.Status().Listening }

func (p *phone) stayAwake(ctx context.Context) {
	var noNap, awake func()
	set := func(on bool, hold *func(), take func(string) func(), reason string) {
		switch {
		case on && *hold == nil && take != nil:
			*hold = take(reason)
		case !on && *hold != nil:
			(*hold)()
			*hold = nil
		}
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		listening := p.on()
		busy := false
		if listening && p.eng.Config().Remote.KeepAwake {
			busy = !p.eng.CheckQuit(ctx).Empty()
		}
		set(listening, &noNap, thisPlatform.NoNap, "Freeloop answers your phone")
		set(busy, &awake, thisPlatform.KeepAwake, "Freeloop: a session is working or waiting for you")
		p.awake.Store(awake != nil)
		select {
		case <-ctx.Done():
			set(false, &noNap, nil, "")
			set(false, &awake, nil, "")
			return
		case <-tick.C:
		}
	}
}
