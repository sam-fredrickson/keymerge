# Show available recipes
help:
    @just --list --unsorted

# Build CLI programs
build:
    go build ./cmd/cfgmerge
    go build ./cmd/cfgmerge-krm

# Lint and format
lint:
    golangci-lint run --fix

# Run all tests
test:
    go test -v -count=1 ./...

# Run all tests with race detection
test-race:
    go test -v -count=1 -race ./...

# Run all tests & generate coverage report
test-cover:
    go test -coverprofile=coverage.out -coverpkg=. .
    go test -coverprofile=cmd/cfgmerge/coverage.out -coverpkg=./cmd/cfgmerge ./cmd/cfgmerge
    go test -coverprofile=cmd/cfgmerge-krm/coverage.out -coverpkg=./cmd/cfgmerge-krm ./cmd/cfgmerge-krm

# View current coverage report
view-coverage:
    go tool cover -func=coverage.out

# View current coverage report as HTML
view-coverage-html:
    go tool cover -html=coverage.out

# Run benchmarks
bench:
    go test -bench=. -benchmem ./bench/...

# Run benchmarks with CPU and memory profiling
profile NAME:
    go test -bench=. -benchmem -cpuprofile=cpu-{{NAME}}.prof -memprofile=mem-{{NAME}}.prof ./bench/...
    @echo "Profiling complete. Use 'just view-profile {{NAME}}' to view results."

# View CPU profile
view-profile NAME TYPE="cpu":
    go tool pprof -http=:8080 {{TYPE}}-{{NAME}}.prof

# View CPU profile in terminal (top 20)
view-profile-text NAME TYPE="cpu":
    go tool pprof -top {{TYPE}}-{{NAME}}.prof

# Compare two CPU profiles
compare-profile NAME1 NAME2 TYPE="cpu":
    go tool pprof -http=:8080 -base={{TYPE}}-{{NAME1}}.prof {{TYPE}}-{{NAME2}}.prof

# Run fuzz tests (default: 30s each)
fuzz TIME="30s":
    @echo "Fuzzing YAML merge..."
    go test -fuzz=FuzzMergeYAML -fuzztime={{TIME}}
    @echo "\nFuzzing direct merge..."
    go test -fuzz=FuzzMergeDirect -fuzztime={{TIME}}
    @echo "\nFuzzing primary key merge..."
    go test -fuzz=FuzzMergeWithPrimaryKeys -fuzztime={{TIME}}
    @echo "\nFuzzing scalar list modes..."
    go test -fuzz=FuzzMergeScalarModes -fuzztime={{TIME}}

# Launch godoc web server
doc:
    go doc -all -http

# Count lines in Go files (regular vs comment lines)
wc:
    #!/usr/bin/env bash
    printf "%-8s %-8s %-8s %-8s %s\n" "regular" "comment" "total" "%//" "file"
    total_regular=0
    total_comment=0
    declare -a lines
    for file in $(find . -type f -name '*.go'); do
        [ -f "$file" ] || continue
        read regular comment < <(awk '
            /^[[:space:]]*\/\// { comment++ }
            !/^[[:space:]]*\/\// { regular++ }
            END { printf "%d %d", regular, comment }
        ' "$file")
        total=$((regular + comment))
        percent=$(awk "BEGIN { printf \"%.1f\", ($comment / $total) * 100 }")
        lines+=("$(printf "%-8d %-8d %-8d %-8s %s" "$regular" "$comment" "$total" "$percent%" "$file")")
        total_regular=$((total_regular + regular))
        total_comment=$((total_comment + comment))
    done
    printf "%s\n" "${lines[@]}" | sort -n
    grand_total=$((total_regular + total_comment))
    total_percent=$(awk "BEGIN { printf \"%.1f\", ($total_comment / $grand_total) * 100 }")
    printf "%-8d %-8d %-8d %-8s %s\n" "$total_regular" "$total_comment" "$grand_total" "$total_percent%" "total"
