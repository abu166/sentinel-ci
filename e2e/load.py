"""Bounded read-only gateway latency probe; reports measurements, not estimates."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import json
import statistics
import time
from simulate import request


def sample(_):
    start = time.perf_counter()
    request("/api/v1/metrics/overview")
    return (time.perf_counter() - start) * 1000


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--requests", type=int, default=100)
    parser.add_argument("--concurrency", type=int, default=8)
    args = parser.parse_args()
    if not 1 <= args.requests <= 10000 or not 1 <= args.concurrency <= 64:
        parser.error("requests must be 1..10000 and concurrency 1..64")
    start = time.perf_counter()
    with ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        values = sorted(pool.map(sample, range(args.requests)))
    print(json.dumps({"requests": len(values), "concurrency": args.concurrency,
                      "requests_per_second": len(values) / (time.perf_counter() - start),
                      "median_ms": statistics.median(values),
                      "p95_ms": values[min(len(values) - 1, int(len(values) * .95))]}, indent=2))
