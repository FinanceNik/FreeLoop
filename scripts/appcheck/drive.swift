// Loads a page in a WebKit view set up as the app's is, with no dialogs, runs a scenario
// in it and prints its results. Exits 1 if a step failed, 2 if it could not run.
import Cocoa
import WebKit

// Like Wails' delegate, this one implements no JavaScript dialogs.
final class UI: NSObject, WKUIDelegate {}

// Key without the app being active, so real mouse events reach the page and the user's
// focus stays where it is.
final class KeyWindow: NSWindow {
  override var canBecomeKey: Bool { true }
}

// Real mouse events, for what the page's own synthetic ones cannot do, such as changing a
// selection: the scenario sends [x, y, clicks], [x, y, 1, toX, toY] for a drag, or
// [x, y, -1] for a right click.
final class Mouse: NSObject, WKScriptMessageHandlerWithReply {
  func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping (Any?, String?) -> Void) {
    guard let a = message.body as? [Double], a.count >= 3 else { return replyHandler(nil, "bad mouse message") }
    Task { @MainActor in
      let point = { (x: Double, y: Double) in NSPoint(x: x, y: web.frame.height - y) }
      func send(_ type: NSEvent.EventType, _ p: NSPoint, _ clicks: Int) async {
        let event = NSEvent.mouseEvent(with: type, location: p, modifierFlags: [], timestamp: ProcessInfo.processInfo.systemUptime, windowNumber: window.windowNumber, context: nil, eventNumber: 1, clickCount: clicks, pressure: 1)!
        window.sendEvent(event)
        try? await Task.sleep(nanoseconds: 40_000_000)
      }
      let from = point(a[0], a[1])
      if a[2] < 0 {
        await send(.rightMouseDown, from, 1)
        await send(.rightMouseUp, from, 1)
        return replyHandler(true, nil)
      }
      for click in 1...Int(a[2]) {
        await send(.leftMouseDown, from, click)
        if a.count == 5 {
          let to = point(a[3], a[4])
          await send(.leftMouseDragged, NSPoint(x: (from.x + to.x) / 2, y: (from.y + to.y) / 2), click)
          await send(.leftMouseDragged, to, click)
          await send(.leftMouseUp, to, click)
        } else {
          await send(.leftMouseUp, from, click)
        }
      }
      replyHandler(true, nil)
    }
  }
}

final class Driver: NSObject, WKNavigationDelegate {
  let scenario: String
  init(scenario: String) { self.scenario = scenario }

  func webView(_ web: WKWebView, didFinish navigation: WKNavigation!) {
    Task { @MainActor in
      do {
        let result = try await web.callAsyncJavaScript(scenario, contentWorld: .page)
        let lines = (result as? [[String: Any]]) ?? []
        var failed = false
        for line in lines {
          let ok = line["ok"] as? Bool ?? false
          failed = failed || !ok
          print("\(ok ? "ok  " : "FAIL")  \(line["name"] ?? "")\(ok ? "" : ": \(line["detail"] ?? "")")")
        }
        exit(lines.isEmpty ? 2 : failed ? 1 : 0)
      } catch {
        print("the scenario did not run: \(error)")
        exit(2)
      }
    }
  }

  func webView(_ web: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
    print("the page did not load: \(error)")
    exit(2)
  }
}

let args = CommandLine.arguments
guard args.count == 3, let url = URL(string: args[1]), let scenario = try? String(contentsOfFile: args[2], encoding: .utf8) else {
  print("usage: drive <url> <scenario.js>")
  exit(2)
}
let app = NSApplication.shared
let window = KeyWindow(contentRect: NSRect(x: 0, y: 0, width: 1400, height: 720), styleMask: [.borderless], backing: .buffered, defer: false)
let mouse = Mouse()
let configuration = WKWebViewConfiguration()
configuration.userContentController.addScriptMessageHandler(mouse, contentWorld: .page, name: "mouse")
let web = WKWebView(frame: window.contentView!.bounds, configuration: configuration)
window.contentView!.addSubview(web)
window.makeKeyAndOrderFront(nil)
let ui = UI()
let driver = Driver(scenario: scenario)
web.uiDelegate = ui
web.navigationDelegate = driver
web.load(URLRequest(url: url))
DispatchQueue.main.asyncAfter(deadline: .now() + 240) {
  print("the scenario took too long")
  exit(2)
}
app.run()
