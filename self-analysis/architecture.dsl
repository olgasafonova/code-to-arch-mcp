workspace {
    model {
        system = softwareSystem "Code-to-Arch MCP - Self Analysis" {
            pkg___code_to_arch__main = container "main" "" "go"
            pkg___common__common = container "common" "" "go"
            pkg___detector__detector = container "detector" "" "go"
            pkg___drift__drift = container "drift" "" "go"
            pkg___golang__golang = container "golang" "" "go"
            pkg___infra__infra = container "infra" "" "go"
            pkg___model__model = container "model" "" "go"
            pkg___python__python = container "python" "" "go"
            pkg___render__render = container "render" "" "go"
            pkg___safepath__safepath = container "safepath" "" "go"
            pkg___scanner__scanner = container "scanner" "" "go"
            pkg___tools__tools = container "tools" "" "go"
            pkg___tracing__tracing = container "tracing" "" "go"
            pkg___typescript__typescript = container "typescript" "" "go"
        }
        pkg___code_to_arch__main -> pkg___tools__tools "github.com/olgasafonova/code-to-arch-mcp/tools"
        pkg___code_to_arch__main -> pkg___tracing__tracing "github.com/olgasafonova/code-to-arch-mcp/tracing"
        pkg___detector__detector -> pkg___model__model "github.com/olgasafonova/code-to-arch-mcp/internal/model"
        pkg___golang__golang -> pkg___common__common "github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/common"
        pkg___golang__golang -> pkg___scanner__scanner "github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
        pkg___typescript__typescript -> pkg___common__common "github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/common"
        pkg___typescript__typescript -> pkg___scanner__scanner "github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
        pkg___common__common -> pkg___model__model "github.com/olgasafonova/code-to-arch-mcp/internal/model"
        pkg___drift__drift -> pkg___model__model "github.com/olgasafonova/code-to-arch-mcp/internal/model"
        pkg___render__render -> pkg___model__model "github.com/olgasafonova/code-to-arch-mcp/internal/model"
        pkg___scanner__scanner -> pkg___model__model "github.com/olgasafonova/code-to-arch-mcp/internal/model"
        pkg___python__python -> pkg___common__common "github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/common"
        pkg___python__python -> pkg___scanner__scanner "github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
        pkg___tools__tools -> pkg___golang__golang "github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/golang"
        pkg___tools__tools -> pkg___python__python "github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/python"
        pkg___tools__tools -> pkg___typescript__typescript "github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/typescript"
        pkg___tools__tools -> pkg___detector__detector "github.com/olgasafonova/code-to-arch-mcp/internal/detector"
        pkg___tools__tools -> pkg___drift__drift "github.com/olgasafonova/code-to-arch-mcp/internal/drift"
        pkg___tools__tools -> pkg___infra__infra "github.com/olgasafonova/code-to-arch-mcp/internal/infra"
        pkg___tools__tools -> pkg___render__render "github.com/olgasafonova/code-to-arch-mcp/internal/render"
        pkg___tools__tools -> pkg___safepath__safepath "github.com/olgasafonova/code-to-arch-mcp/internal/safepath"
    }
    views {
        container system {
            include *
            autoLayout
        }
    }
}
