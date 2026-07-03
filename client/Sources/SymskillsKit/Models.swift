import Foundation

public struct Config: Codable, Equatable, Sendable {
    public let libraryDir: String
    public let renderDir: String
    public let cacheDir: String

    enum CodingKeys: String, CodingKey {
        case libraryDir = "library_dir"
        case renderDir = "render_dir"
        case cacheDir = "cache_dir"
    }
}

public struct TargetPath: Codable, Equatable, Sendable {
    public let target: String
    public let user: String
}

public struct DoctorInfo: Codable, Equatable, Sendable {
    public let configPath: String
    public let config: Config
    public let targets: [TargetPath]

    enum CodingKeys: String, CodingKey {
        case configPath = "config_path"
        case config
        case targets
    }
}

public struct SkillSummary: Codable, Identifiable, Equatable, Hashable, Sendable {
    public var id: String { path }
    public let name: String
    public let description: String
    public let path: String
}

public struct ImportResult: Codable, Equatable, Sendable {
    public let name: String
    public let path: String
}

public struct SkillListResult: Codable, Equatable, Sendable {
    public let skills: [SkillSummary]
    public let issues: [Issue]
}

public struct Issue: Codable, Identifiable, Equatable, Sendable {
    public var id: String { "\(code)-\(path ?? "")-\(message)" }
    public let code: String
    public let severity: String
    public let message: String
    public let path: String?
}

public struct Frontmatter: Codable, Equatable, Sendable {
    public let name: String
    public let description: String
    public let version: String?
    public let author: String?
    public let license: String?
    public let compatibility: String?
    public let platforms: [String]?
    public let requiredEnvironmentVariables: [String]?
    public let metadata: [String: JSONValue]?

    enum CodingKeys: String, CodingKey {
        case name
        case description
        case version
        case author
        case license
        case compatibility
        case platforms
        case requiredEnvironmentVariables = "required_environment_variables"
        case metadata
    }
}

public struct ManifestSkill: Codable, Equatable, Sendable {
    public let name: String
    public let version: String
    public let source: String
}

public struct TargetConfig: Codable, Equatable, Sendable {
    public let enabled: Bool
    public let alias: String?
    public let description: String?
    public let scope: String?
    public let category: String?
    public let prepend: String?
    public let append: String?
    public let metadata: [String: String]?
}

public struct Manifest: Codable, Equatable, Sendable {
    public let skill: ManifestSkill
    public let targets: [String: TargetConfig]?
}

public struct SkillBundle: Codable, Equatable, Sendable {
    public let root: String
    public let frontmatter: Frontmatter
    public let manifest: Manifest
    public let body: String
}

public struct Rendered: Codable, Identifiable, Equatable, Sendable {
    public var id: String { target }
    public let target: String
    public let name: String
    public let path: String?
    public let frontmatter: Frontmatter
    public let skillMd: String?

    enum CodingKeys: String, CodingKey {
        case target
        case name
        case path
        case frontmatter
        case skillMd = "skill_md"
    }
}

public struct InstallResult: Codable, Equatable, Sendable {
    public let action: String
    public let target: String
    public let name: String
    public let path: String
    public let mode: String
}

public struct Change: Codable, Identifiable, Equatable, Sendable {
    public var id: String { path }
    public let path: String
    public let status: String
}

// A dynamic JSON value parser for Frontmatter.metadata.
public enum JSONValue: Codable, Equatable, Sendable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case object([String: JSONValue])
    case array([JSONValue])
    case null

    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if let x = try? container.decode(String.self) {
            self = .string(x)
        } else if let x = try? container.decode(Double.self) {
            self = .number(x)
        } else if let x = try? container.decode(Bool.self) {
            self = .bool(x)
        } else if let x = try? container.decode([String: JSONValue].self) {
            self = .object(x)
        } else if let x = try? container.decode([JSONValue].self) {
            self = .array(x)
        } else if container.decodeNil() {
            self = .null
        } else {
            throw DecodingError.typeMismatch(JSONValue.self, DecodingError.Context(codingPath: decoder.codingPath, debugDescription: "Wrong type for JSONValue"))
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let x): try container.encode(x)
        case .number(let x): try container.encode(x)
        case .bool(let x): try container.encode(x)
        case .object(let x): try container.encode(x)
        case .array(let x): try container.encode(x)
        case .null: try container.encodeNil()
        }
    }
    
    public var stringValue: String? {
        if case .string(let s) = self { return s }
        return nil
    }
}
