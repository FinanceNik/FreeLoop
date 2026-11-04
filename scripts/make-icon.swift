// Draws build/appicon.png, the app's icon: the mark (three circles in a triangle, as in
// tools/make_logo.py of the author's website, with the same numbers) on a white tile in
// the proportions of a macOS icon. scripts/make-icon.sh runs it and makes the .icns.
import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

let size = 1024.0
let tile = 824.0 // macOS icons leave a margin for the shadow
let radius = 185.0

let circleRadius = 16.5, spread = 11.5, opacity = 0.7
let colours: [(Double, Double, Double)] = [(109, 202, 222), (125, 160, 238), (243, 220, 128)] // cyan, blue, yellow
let angles = [-90.0, 150.0, 30.0] // top, bottom left, bottom right; yellow lies on top

let space = CGColorSpace(name: CGColorSpace.sRGB)!
let c = CGContext(data: nil, width: Int(size), height: Int(size), bitsPerComponent: 8, bytesPerRow: 0, space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
c.translateBy(x: 0, y: size)
c.scaleBy(x: 1, y: -1)

let margin = (size - tile) / 2
let shape = CGPath(roundedRect: CGRect(x: margin, y: margin, width: tile, height: tile), cornerWidth: radius, cornerHeight: radius, transform: nil)
c.saveGState()
// A shadow's offset ignores the flip above, so down is negative here.
c.setShadow(offset: CGSize(width: 0, height: -12), blur: 28, color: CGColor(srgbRed: 0, green: 0, blue: 0, alpha: 0.3))
c.setFillColor(CGColor(srgbRed: 1, green: 1, blue: 1, alpha: 1))
c.addPath(shape)
c.fillPath()
c.restoreGState()

// The mark fills the tile as far as its circles' reach leaves room.
let scale = tile * 0.8 / (2 * (spread + circleRadius))
let middle = size / 2
for (angle, colour) in zip(angles, colours) {
  let a = angle * .pi / 180
  let x = middle + (spread * cos(a)) * scale
  let y = middle + (spread * sin(a) + spread / 4) * scale
  let r = circleRadius * scale
  c.setFillColor(CGColor(srgbRed: colour.0 / 255, green: colour.1 / 255, blue: colour.2 / 255, alpha: opacity))
  c.fillEllipse(in: CGRect(x: x - r, y: y - r, width: 2 * r, height: 2 * r))
}

let out = URL(fileURLWithPath: CommandLine.arguments[1]) as CFURL
let file = CGImageDestinationCreateWithURL(out, UTType.png.identifier as CFString, 1, nil)!
CGImageDestinationAddImage(file, c.makeImage()!, nil)
guard CGImageDestinationFinalize(file) else { fatalError("could not write the icon") }
