import SwiftUI

@main
struct SymskillsApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
                .preferredColorScheme(.dark) // Lock dark mode for brand aesthetics
        }
        .windowStyle(.hiddenTitleBar) // Modern title bar look
    }
}
