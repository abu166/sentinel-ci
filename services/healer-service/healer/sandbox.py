"""Disposable Docker verification jobs; source is transferred over stdin, never mounts."""
import asyncio
from contextlib import suppress
import io
import os
import tarfile
from uuid import uuid4


def archive(files: dict[str, str]) -> bytes:
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w") as tar:
        for name, content in files.items():
            data = content.encode()
            member = tarfile.TarInfo(name)
            member.size = len(data)
            member.mode = 0o600
            member.uid = member.gid = 65532
            tar.addfile(member, io.BytesIO(data))
    return output.getvalue()


def arguments(name: str, command: str) -> list[str]:
    return ["docker", "run", "--rm", "--name", name, "-i", "--read-only", "--network", "none", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--memory", os.getenv("SANDBOX_MEMORY", "768m"), "--cpus", os.getenv("SANDBOX_CPUS", "1"), "--pids-limit", "128", "--tmpfs", "/workspace:rw,exec,uid=65532,gid=65532,size=512m", "--tmpfs", "/tmp:rw,exec,uid=65532,gid=65532,size=256m", "--user", "65532:65532", "-e", "HOME=/tmp", "-e", "GOCACHE=/tmp/gocache", "-e", "GOPATH=/tmp/gopath", "-e", "GOTOOLCHAIN=local", "-e", "GOPROXY=off", "-e", "GOMAXPROCS=2", "-e", "GOFLAGS=-p=2", "-e", "PYTHONDONTWRITEBYTECODE=1", os.getenv("SANDBOX_IMAGE", "sentinel-sandbox:local"), "sh", "-c", 'tar -xf - -C /workspace && cd /workspace && exec sh -c "$1"', "sh", command]


async def remove(name: str) -> None:
    with suppress(OSError, TimeoutError):
        process = await asyncio.create_subprocess_exec("docker", "rm", "-f", name, stdout=asyncio.subprocess.DEVNULL, stderr=asyncio.subprocess.DEVNULL)
        await asyncio.wait_for(process.wait(), 10)


async def run(files: dict[str, str], command: str, timeout: int) -> tuple[int, str]:
    name = "sentinel-verify-" + uuid4().hex
    try:
        process = await asyncio.create_subprocess_exec(*arguments(name, command), stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.STDOUT)
        assert process.stdin is not None and process.stdout is not None
        captured = bytearray()
        async def write() -> None:
            try:
                process.stdin.write(archive(files))
                await process.stdin.drain()
            except (BrokenPipeError, ConnectionResetError):
                return
            finally:
                process.stdin.close()
        async def read() -> None:
            while chunk := await process.stdout.read(8192):
                captured.extend(chunk[:max(0, 1_000_000-len(captured))])
        async def complete() -> None:
            await asyncio.gather(write(), read(), process.wait())
        try:
            await asyncio.wait_for(complete(), timeout)
        except TimeoutError:
            await remove(name)
            await process.wait()
            return 124, captured.decode(errors="replace") + "\nVerification timed out"
        except asyncio.CancelledError:
            await remove(name)
            await process.wait()
            raise
        return process.returncode or 0, captured.decode(errors="replace")
    finally:
        await remove(name)
