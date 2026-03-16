import os
import re
import subprocess


def get_badge(percentage_str):
    try:
        p = float(percentage_str.strip("%"))
        if p >= 70:
            color = "brightgreen"
        elif p >= 50:
            color = "yellow"
        else:
            color = "red"
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
        avg_pct = sum(file_data[path]) / len(file_data[path])
        avg_str = f"{avg_pct:.1f}%"
        badge = get_badge(avg_str)
        table.append(f"| `{path}` | {badge} |")

    total_badge = get_badge(total_coverage)
    table.append(f"| **Total** | {total_badge} |")

    table_content = "\n".join(table) + "\n"
    table_content += "\n> Untested code not shown above is either auto-generated, boilerplate delegation, or covered by E2E.\n"

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
