#!/usr/bin/env python3
"""
Monitor a Docker container's CPU/Mem/IO/Net using `docker stats`.

Usage:
  python3 monitor_container.py --container neo4j --interval 2 --out neo4j_stats.csv
  python3 monitor_container.py --container <container_id> --interval 1
"""

import argparse
import csv
import datetime as dt
import os
import subprocess
import sys
import time

DOCKER_STATS_FMT = (
    "{{.Container}},{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.MemPerc}},"
    "{{.NetIO}},{{.BlockIO}},{{.PIDs}}"
)


def run(cmd):
    return subprocess.check_output(cmd, stderr=subprocess.STDOUT, text=True).strip()


def docker_exists():
    try:
        run(["docker", "version"])
        return True
    except Exception:
        return False


def parse_line(line):
    # line: container,name,cpu,memUsage,memPerc,netIO,blockIO,pids
    parts = [p.strip() for p in line.split(",")]
    if len(parts) != 8:
        return None
    return {
        "container": parts[0],
        "name": parts[1],
        "cpu_perc": parts[2],
        "mem_usage": parts[3],
        "mem_perc": parts[4],
        "net_io": parts[5],
        "block_io": parts[6],
        "pids": parts[7],
    }


def format_log_line(ts, data):
    return (
        f"[{ts}] {data['name']} "
        f"CPU {data['cpu_perc']} | MEM {data['mem_usage']} ({data['mem_perc']}) | "
        f"NET {data['net_io']} | IO {data['block_io']} | PIDs {data['pids']}"
    )


def prune_log(path, max_lines, max_bytes):
    if not os.path.exists(path):
        return

    try:
        if max_lines > 0:
            with open(path, "r", encoding="utf-8") as fp:
                lines = fp.readlines()
            if len(lines) > max_lines:
                lines = lines[-max_lines:]
                with open(path, "w", encoding="utf-8") as fp:
                    fp.writelines(lines)

        if max_bytes > 0:
            size = os.path.getsize(path)
            if size > max_bytes:
                with open(path, "rb") as fp:
                    fp.seek(max(0, size - max_bytes))
                    data = fp.read()
                # Drop partial first line for readability
                if b"\n" in data:
                    data = data.split(b"\n", 1)[1]
                with open(path, "wb") as fp:
                    fp.write(data)
    except Exception:
        # Best-effort pruning; do not crash the monitor.
        return


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--container", required=True, help="Container name or ID (e.g. neo4j)")
    ap.add_argument("--interval", type=float, default=2.0, help="Seconds between samples")
    ap.add_argument("--out", default="", help="CSV output file (optional)")
    ap.add_argument("--log", default="", help="Human-readable log file (optional)")
    ap.add_argument("--max-lines", type=int, default=0, help="Max log lines (0 = unlimited)")
    ap.add_argument("--max-bytes", type=int, default=0, help="Max log file size in bytes (0 = unlimited)")
    ap.add_argument("--count", type=int, default=0, help="Number of samples (0 = run forever)")
    ap.add_argument("--max-failures", type=int, default=0,
                     help="Exit after N consecutive docker stats failures (0 = retry forever)")
    args = ap.parse_args()

    if not docker_exists():
        print("ERROR: docker CLI not found or not running.", file=sys.stderr)
        sys.exit(1)

    out_fp = None
    writer = None
    log_fp = None
    if args.out:
        out_fp = open(args.out, "w", newline="")
        writer = csv.DictWriter(
            out_fp,
            fieldnames=[
                "ts",
                "container",
                "name",
                "cpu_perc",
                "mem_usage",
                "mem_perc",
                "net_io",
                "block_io",
                "pids",
            ],
        )
        writer.writeheader()
    if args.log:
        os.makedirs(os.path.dirname(args.log) or ".", exist_ok=True)
        log_fp = open(args.log, "a", encoding="utf-8")

    i = 0
    consecutive_failures = 0
    try:
        while True:
            ts = dt.datetime.utcnow().isoformat() + "Z"
            try:
                line = run(
                    [
                        "docker",
                        "stats",
                        "--no-stream",
                        "--format",
                        DOCKER_STATS_FMT,
                        args.container,
                    ]
                )
                consecutive_failures = 0  # Reset on success
            except subprocess.CalledProcessError as e:
                consecutive_failures += 1
                print(
                    f"[{ts}] ERROR running docker stats ({consecutive_failures}x): "
                    f"{e.output.strip()}",
                    file=sys.stderr,
                )
                if args.max_failures > 0 and consecutive_failures >= args.max_failures:
                    msg = (
                        f"[{ts}] FATAL: container '{args.container}' unreachable after "
                        f"{consecutive_failures} consecutive failures — exiting"
                    )
                    print(msg, file=sys.stderr)
                    if log_fp:
                        log_fp.write(msg + "\n")
                        log_fp.flush()
                    sys.exit(1)
                time.sleep(args.interval)
                continue

            data = parse_line(line)
            if not data:
                print(f"[{ts}] WARN: unexpected output: {line}", file=sys.stderr)
                time.sleep(args.interval)
                continue

            # Print a compact status line
            line = format_log_line(ts, data)
            print(line)

            if writer:
                row = {"ts": ts, **data}
                writer.writerow(row)
                out_fp.flush()
            if log_fp:
                log_fp.write(line + "\n")
                log_fp.flush()
                prune_log(args.log, args.max_lines, args.max_bytes)

            i += 1
            if args.count and i >= args.count:
                break

            time.sleep(args.interval)
    finally:
        if out_fp:
            out_fp.close()
        if log_fp:
            log_fp.close()


if __name__ == "__main__":
    main()
