// Loads a page in a WebKit view set up as the app's is, with no dialogs, runs a scenario
// in it and prints its results. Exits 1 if a step failed, 2 if it could not run.
import Cocoa
import WebKit

// Like Wails' delegate, this one implements no JavaScript dialogs.
final class UI: NSObject, WKUIDelegate {}

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
let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1400, height: 720), styleMask: [.borderless], backing: .buffered, defer: false)
let web = WKWebView(frame: window.contentView!.bounds)
window.contentView!.addSubview(web)
window.orderBack(nil)
let ui = UI()
let driver = Driver(scenario: scenario)
web.uiDelegate = ui
web.navigationDelegate = driver
web.load(URLRequest(url: url))
DispatchQueue.main.asyncAfter(deadline: .now() + 120) {
  print("the scenario took too long")
  exit(2)
}
app.run()
