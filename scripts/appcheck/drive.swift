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

// Takes a click even with the app not active, which it often is not while a check runs in
// the background: a view in an inactive window drops mouse events without a word.
final class CheckedWebView: WKWebView {
  override func acceptsFirstMouse(for event: NSEvent?) -> Bool { true }
}

// Real mouse events, for what the page's own synthetic ones cannot do, such as changing a
// selection: the scenario sends [x, y, clicks], [x, y, 1, toX, toY] for a drag, or
// [x, y, -1] for a right click.
final class Mouse: NSObject, WKScriptMessageHandlerWithReply {
  func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping (Any?, String?) -> Void) {
    guard let a = message.body as? [Double], a.count >= 3 else { return replyHandler(nil, "bad mouse message") }
    Task { @MainActor in
      window.makeKeyAndOrderFront(nil)
      window.makeFirstResponder(web)
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

// A picture of the window, for a check that failed: on a machine no one watches, such as
// CI, it shows what the page looked like. The scenario sends a name; the picture goes to
// $APPCHECK_SHOTS, numbered, when that is set.
final class Snap: NSObject, WKScriptMessageHandlerWithReply {
  var count = 0
  func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping (Any?, String?) -> Void) {
    guard let dir = ProcessInfo.processInfo.environment["APPCHECK_SHOTS"], !dir.isEmpty else { return replyHandler(false, nil) }
    let name = String((message.body as? String ?? "check").map { $0.isLetter || $0.isNumber ? $0 : "-" }.prefix(60))
    count += 1
    let file = URL(fileURLWithPath: dir).appendingPathComponent(String(format: "%02d-%@.png", count, name))
    Task { @MainActor in
      web.takeSnapshot(with: nil) { image, _ in
        if let tiff = image?.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff), let png = rep.representation(using: .png, properties: [:]) {
          try? png.write(to: file)
        }
        replyHandler(true, nil)
      }
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
          let detail = line["detail"] as? String ?? ""
          print("\(ok ? "ok  " : "FAIL")  \(line["name"] ?? "")\(ok && (detail.isEmpty || !(line["name"] as? String ?? "").hasPrefix("DIAGNOSIS")) ? "" : ": \(detail)")")
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
let snap = Snap()
configuration.userContentController.addScriptMessageHandler(snap, contentWorld: .page, name: "snap")
let web = CheckedWebView(frame: window.contentView!.bounds, configuration: configuration)
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
