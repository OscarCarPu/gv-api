import os
import re
import subprocess

# Paths to hide entirely from the coverage table
HIDDEN_PATHS = {
    "/internal/database/habitsdb/",
    "/internal/database/tasksdb/",
    "/internal/database/sqlc/",
    "/cmd/api/main.go",
    "/internal/config/config.go",
    "/internal/database/db.go",
    "/test/e2e/client.go",
    "/test/e2e/setup.go",
}


def is_hidden(path):
    return any(h in path for h in HIDDEN_PATHS)


def get_badge(percentage_str):
    try:
        p = float(percentage_str.strip("%"))
        if p >= 80:
            color = "brightgreen"
        elif p >= 50:
            color = "yellow"
        else:
            color = "red"
        # URL encode the % as %25
        return f"![{p}%](https://img.shields.io/badge/{p}%25-{color})"
    except ValueError:
        return "![0%](https://img.shields.io/badge/0%25-red)"


def main():
    try:
        result = subprocess.run(
            ["go", "tool", "cover", "-func=coverage.out"],
            capture_output=True,
            text=True,
            check=True,
        )
        lines = result.stdout.splitlines()
    except Exception as e:
        print(f"Error: {e}")
        return

    file_data = {}
    total_coverage = "0.0%"

    for line in lines:
        fields = line.split()
        if not fields or len(fields) < 3:
            continue

        if fields[0] == "total:":
            total_coverage = fields[2]
            continue

        # Clean the path: remove line numbers like ":12:"
        raw_path = re.sub(r":\d+:$", "", fields[0])

        if raw_path not in file_data:
            file_data[raw_path] = []

        try:
            file_data[raw_path].append(float(fields[2].strip("%")))
        except ValueError:
            continue

    table = [
        "## Coverage\n",
        "| File | Coverage |",
        "| :--- | :---: |",
    ]

    for path in sorted(file_data.keys()):
        if is_hidden(path):
            continue

        avg_pct = sum(file_data[path]) / len(file_data[path])
        avg_str = f"{avg_pct:.1f}%"
        badge = get_badge(avg_str)
        table.append(f"| `{path}` | {badge} |")

    included_pcts = []
    for path, pcts in file_data.items():
        if not is_hidden(path):
            included_pcts.extend(pcts)

    if included_pcts:
        real_total = f"{sum(included_pcts) / len(included_pcts):.1f}%"
    else:
        real_total = total_coverage

    total_badge = get_badge(real_total)
    table.append(f"| **Total** | {total_badge} |")
    table.append("")
    table.append("> Untested code not shown above is either auto-generated (sqlc) or boilerplate that doesn't warrant testing.")

    table_content = "\n".join(table) + "\n"

    if not os.path.exists("README.md"):
        return

    with open("README.md", "r") as f:
        content = f.read()

    header = "## Coverage"
    start_idx = content.find(header)
    if start_idx == -1:
        return

    rest = content[start_idx + len(header) :]
    next_header_idx = rest.find("\n## ")

    if next_header_idx == -1:
        new_readme = content[:start_idx] + table_content + "\n"
    else:
        new_readme = (
            content[:start_idx] + table_content + "\n" + rest[next_header_idx + 1 :]
        )

    with open("README.md", "w") as f:
        f.write(new_readme)


if __name__ == "__main__":
    main()
