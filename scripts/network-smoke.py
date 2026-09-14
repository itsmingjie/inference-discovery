"""Opt-in, same-machine multicast/HTTP lifecycle smoke test; never ordinary CI.

Build bin/inference and the examples/local-provider/mock.go binary first.
Usage: python3 scripts/network-smoke.py --interface en0 --binary bin/inference --mock bin/mock
This does not establish two-machine or Avahi interoperability.
"""
import argparse
import json
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--interface", required=True)
parser.add_argument("--binary", default="bin/inference")
parser.add_argument("--mock", default="bin/mock")
args = parser.parse_args()
binary, mock = str(Path(args.binary).resolve()), str(Path(args.mock).resolve())
name = "Smoke-" + uuid.uuid4().hex[:8]
children = []
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def spawn(cmd):
    p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    children.append(p)
    return p


def stop(p):
    if p.poll() is None:
        p.send_signal(signal.SIGINT)
        try:
            p.wait(timeout=8)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait()


def snapshot():
    result = subprocess.run([binary, "discover", "--json", "--interface", args.interface,
                             "--browse-timeout", "4s"], capture_output=True, text=True, timeout=20)
    assert result.returncode == 0, result.stderr
    return [p for p in json.loads(result.stdout) if p["name"] == name]


def expect(count):
    deadline = time.monotonic() + 35
    while time.monotonic() < deadline:
        records = snapshot()
        if len(records) == count and all("descriptor" in p for p in records):
            return records
    raise AssertionError(f"expected {count} usable records, got {records}")


try:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        api_port = s.getsockname()[1]
    api = spawn([mock, "--listen", f"127.0.0.1:{api_port}"])
    endpoint = f"http://127.0.0.1:{api_port}/v1"
    for _ in range(50):
        try:
            opener.open(endpoint + "/models", timeout=1).close()
            break
        except urllib.error.URLError:
            time.sleep(0.1)
    else:
        raise AssertionError("mock API did not start")
    cmd = [binary, "advertise", "--name", name, "--endpoint", endpoint,
           "--allow-loopback", "--interface", args.interface, "--health-interval", "1s", "--timeout", "1s"]
    first = spawn(cmd)
    records = expect(1)
    # Test advertised descriptor addresses and IPv6 loopback transport separately.
    port = urllib.parse.urlsplit(records[0]["descriptor_url"]).port
    with opener.open(f"http://[::1]:{port}/.well-known/inference.json", timeout=3) as r:
        assert json.load(r)["name"] == name
    second = spawn(cmd)
    records = expect(2)
    assert len({p["id"] for p in records}) == 2, "duplicate DNS names were not resolved"
    chat = subprocess.run([binary, "chat", "--interface", args.interface, "--provider", records[0]["id"]],
                          input="hello\n/quit\n", capture_output=True, text=True, timeout=25)
    assert chat.returncode == 0 and "deterministic demo endpoint" in chat.stdout, chat.stdout + chat.stderr
    print("PASS: multicast discovery, duplicate instances, model selection, streamed mock chat, IPv6 metadata", flush=True)
    stop(second)
    expect(1)
    stop(api)
    expect(0)
    try:
        opener.open(f"http://127.0.0.1:{port}/.well-known/inference.json", timeout=3)
        raise AssertionError("unhealthy metadata remained available")
    except urllib.error.HTTPError as e:
        assert e.code == 503
    api = spawn([mock, "--listen", f"127.0.0.1:{api_port}"])
    expect(1)
    stop(first)
    expect(0)
    print("PASS: clean removal, health withdrawal/503, recovery and final removal", flush=True)
finally:
    failed = sys.exc_info()[0] is not None
    for child in reversed(children):
        stop(child)
        if failed:
            print(f"Process {child.args}: exit {child.returncode}", file=sys.stderr)
            print(child.stderr.read(32768), file=sys.stderr)
