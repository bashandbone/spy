// Swift sample
import Foundation

final class Counter {
    private(set) var value: Int = 0
    func increment(_ n: Int = 1) -> Int {
        value += n
        return value
    }
}

@main
struct App {
    static func main() throws {
        let url = URL(fileURLWithPath: "input.txt")
        let text = try String(contentsOf: url, encoding: .utf8)
        let counter = Counter()
        for line in text.split(separator: "\n") {
            if !line.isEmpty {
                _ = counter.increment(line.count)
            }
        }
        print("total bytes: \(counter.value)")
    }
}
