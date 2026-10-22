"""Verify release binaries, platform manifests, archives, and catalog agree."""
import argparse
import hashlib
import json
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def checksum(data):
    return hashlib.sha256(data).hexdigest()


def check_sums(path, expected):
    actual = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        digest, name = line.split()
        if name in actual:
            raise ValueError(f"Duplicate checksum for {name}")
        actual[name] = digest
    if actual != expected:
        raise ValueError(f"{path.name} does not match the release assets")


def verify(dist, source):
    index = json.loads((dist / "index.json").read_text(encoding="utf-8"))
    if len(index["plugins"]) != 1:
        raise ValueError("Expected one plugin in the release catalog")
    package = index["plugins"][0]
    for key in ("plugin_id", "version"):
        if package["manifest"][key] != source[key]:
            raise ValueError(f"Catalog {key} differs from source manifest")
    repo = source["presentation"]["source_url"]
    base = f"{repo}/releases/download/v{source['version']}"
    if package["repo_url"] != repo or package["checksums_url"] != base + "/checksums.txt":
        raise ValueError("Catalog must use the source repository's release URLs")
    binaries, archives = {}, {}
    for platform in source["supported_platforms"]:
        target_os, arch = platform["os"], platform["arch"]
        platform_key = f"{target_os}/{arch}"
        binary_name = f"plugin-{target_os}-{arch}"
        binary = (dist / binary_name).read_bytes()
        # Validate executable headers without running cross-compiled binaries.
        if target_os == "linux":
            machine = {"amd64": 62, "arm64": 183}[arch]
            valid = (len(binary) >= 20 and binary[:6] == b"\x7fELF\x02\x01"
                     and int.from_bytes(binary[18:20], "little") == machine)
        else:
            valid = (target_os == "darwin" and arch == "arm64" and len(binary) >= 8
                     and binary[:4] == b"\xcf\xfa\xed\xfe"
                     and int.from_bytes(binary[4:8], "little") == 0x0100000C)
        if not valid:
            raise ValueError(f"Wrong executable format for {platform_key}")
        digest = checksum(binary)
        binaries[binary_name] = digest
        expected_manifest = dict(source, checksum=digest)
        manifest = json.loads((dist / f"manifest-{target_os}-{arch}.json").read_text(encoding="utf-8"))
        if manifest != expected_manifest:
            raise ValueError(f"Incorrect platform manifest for {platform_key}")
        if package["binaries"][platform_key] != {"url": base + "/" + binary_name, "checksum": digest}:
            raise ValueError(f"Catalog download or checksum differs for {platform_key}")
        archive_name = f"prefetch-{source['version']}-{target_os}-{arch}.zip"
        archives[archive_name] = checksum((dist / archive_name).read_bytes())
        with zipfile.ZipFile(dist / archive_name) as archive:
            if sorted(archive.namelist()) != ["manifest.json", "plugin"]:
                raise ValueError(f"Incorrect ZIP layout in {archive_name}")
            if archive.read("plugin") != binary or json.loads(archive.read("manifest.json")) != manifest:
                raise ValueError(f"ZIP does not match raw release assets for {platform_key}")
            if archive.getinfo("plugin").external_attr >> 16 != 0o100755:
                raise ValueError(f"ZIP plugin is not executable for {platform_key}")
        print(f"{platform_key}: binary, checksum, manifest, archive, and catalog verified")
    if len(package["binaries"]) != len(binaries):
        raise ValueError("Catalog contains unexpected platform binaries")
    check_sums(dist / "checksums.txt", binaries)
    check_sums(dist / "SHA256SUMS", archives)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", type=Path, default=ROOT / "dist")
    args = parser.parse_args()
    verify(args.dist, json.loads((ROOT / "manifest.json").read_text(encoding="utf-8")))
