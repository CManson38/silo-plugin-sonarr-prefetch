"""Build self-describing Silo binaries and uploadable archives. No pip packages needed."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[1]
PLATFORMS = [("linux", "amd64"), ("linux", "arm64"), ("darwin", "arm64")]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    source_manifest = json.loads((ROOT / "manifest.json").read_text(encoding="utf-8"))
    parser.add_argument("--version", default=source_manifest["version"])
    parser.add_argument("--go", default=shutil.which("go") or str(ROOT / ".tools/go/bin/go.exe"))
    args = parser.parse_args()
    if args.version != source_manifest["version"]:
        parser.error("version must match manifest.json")
    output = ROOT / "dist"
    output.mkdir(exist_ok=True)
    env = dict(os.environ, CGO_ENABLED="0", GOWORK="off")
    if (ROOT / ".tools").is_dir():
        env.update(GOPATH=str(ROOT / ".tools/gopath"), GOCACHE=str(ROOT / ".tools/gocache"))
    sums = []
    binary_sums = []
    for target_os, arch in PLATFORMS:
        name = f"prefetch-{args.version}-{target_os}-{arch}"
        target = output / name
        target.mkdir(exist_ok=True)
        binary = target / "plugin"
        target_env = dict(env, GOOS=target_os, GOARCH=arch)
        subprocess.run([args.go, "build", "-trimpath", "-ldflags",
                        f"-s -w -X main.version={args.version}", "-o", str(binary), "."],
                       cwd=ROOT, env=target_env, check=True)
        manifest = dict(source_manifest, checksum=hashlib.sha256(binary.read_bytes()).hexdigest())
        (target / "manifest.json").write_text(json.dumps(manifest, indent=2)+"\n", encoding="utf-8")
        raw_binary = output / f"plugin-{target_os}-{arch}"
        shutil.copyfile(binary, raw_binary)
        raw_binary.chmod(0o755)
        binary_sums.append(f"{manifest['checksum']}  {raw_binary.name}")
        archive = output / f"{name}.zip"
        with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as z:
            for file in [binary, target / "manifest.json"]:
                info = zipfile.ZipInfo(file.name, (2026, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = (0o100755 if file == binary else 0o100644) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                z.writestr(info, file.read_bytes())
        sums.append(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}")
        print(archive)
    (output / "SHA256SUMS").write_text("\n".join(sums)+"\n", encoding="utf-8")
    (output / "checksums.txt").write_text("\n".join(binary_sums)+"\n", encoding="utf-8")
    subprocess.run([args.go, "run", "./cmd/update-index", "-output", "dist/index.json"], cwd=ROOT, env=env, check=True)


if __name__ == "__main__":
    main()
