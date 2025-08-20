#!/usr/bin/env python3
# file: .github/scripts/docker-security.py
# version: 1.0.0
# guid: e2f3g4h5-i6j7-k8l9-m0n1-o2p3q4r5s6t7

"""
Docker security scanning utilities for matrix build system.
Handles vulnerability scanning, SBOM generation, and security reporting.
"""

import json
import subprocess
import sys


def run_command(cmd, capture_output=True, check=True):
    """Run a shell command and return the result."""
    try:
        result = subprocess.run(
            cmd, shell=True, capture_output=capture_output, text=True, check=check
        )
        return result.returncode == 0, result.stdout.strip(), result.stderr.strip()
    except subprocess.CalledProcessError as e:
        return False, e.stdout if e.stdout else "", e.stderr if e.stderr else ""
    except Exception as e:
        return False, "", str(e)


def scan_image_with_trivy(image_ref, output_format="sarif", output_file=None):
    """Scan Docker image with Trivy."""
    if not output_file:
        output_file = f"trivy-results.{output_format}"

    print(f"Scanning image {image_ref} with Trivy...")

    cmd = f"trivy image --format {output_format} --output {output_file} {image_ref}"
    success, stdout, stderr = run_command(cmd, check=False)

    if not success:
        print(f"Trivy scan failed: {stderr}")
        return False, None

    print(f"Trivy scan completed: {output_file}")
    return True, output_file


def scan_filesystem_with_trivy(scan_path=".", output_file="trivy-fs-results.txt"):
    """Scan filesystem with Trivy."""
    print(f"Scanning filesystem {scan_path} with Trivy...")

    cmd = f"trivy fs --format table --output {output_file} {scan_path}"
    success, stdout, stderr = run_command(cmd, check=False)

    if not success:
        print(f"Trivy filesystem scan failed: {stderr}")
        return False, None

    print(f"Trivy filesystem scan completed: {output_file}")
    return True, output_file


def generate_sbom(image_ref, output_file="sbom.spdx.json"):
    """Generate Software Bill of Materials."""
    print(f"Generating SBOM for {image_ref}...")

    # Try syft first (if available), fallback to docker sbom
    cmd = f"syft {image_ref} -o spdx-json > {output_file}"
    success, stdout, stderr = run_command(cmd, check=False)

    if not success:
        # Fallback to docker sbom if syft is not available
        cmd = f"docker sbom {image_ref} --output {output_file}"
        success, stdout, stderr = run_command(cmd, check=False)

    if not success:
        print(f"SBOM generation failed: {stderr}")
        return False, None

    print(f"SBOM generated: {output_file}")
    return True, output_file


def test_image_functionality(image_ref):
    """Test Docker image basic functionality."""
    print(f"Testing image functionality: {image_ref}")

    tests = []

    # Test 1: Container starts
    print("Testing container startup...")
    cmd = f'docker run --rm --entrypoint="" {image_ref} echo "Container start test"'
    success, stdout, stderr = run_command(cmd, check=False)
    tests.append(
        {
            "name": "Container Startup",
            "passed": success,
            "message": "Container starts successfully"
            if success
            else f"Failed: {stderr}",
        }
    )

    # Test 2: Check application files
    print("Checking application files...")
    cmd = f'docker run --rm --entrypoint="" {image_ref} ls /app/ 2>/dev/null || docker run --rm --entrypoint="" {image_ref} ls /usr/local/bin/ 2>/dev/null'
    success, stdout, stderr = run_command(cmd, check=False)
    tests.append(
        {
            "name": "Application Files",
            "passed": success,
            "message": "Application files present"
            if success
            else "Application structure unknown",
        }
    )

    # Test 3: Check health configuration
    print("Checking health configuration...")
    cmd = f'docker inspect {image_ref} --format="{{{{.Config.Healthcheck}}}}"'
    success, stdout, stderr = run_command(cmd, check=False)
    has_healthcheck = (
        success and stdout != "none" and stdout != "<nil>" and stdout.strip() != ""
    )
    tests.append(
        {
            "name": "Health Check",
            "passed": has_healthcheck,
            "message": "Health check configured"
            if has_healthcheck
            else "No health check configured",
        }
    )

    return tests


def validate_compose_files(compose_files):
    """Validate docker-compose files."""
    results = []

    for compose_file in compose_files:
        print(f"Validating {compose_file}...")
        cmd = f"docker-compose -f {compose_file} config"
        success, stdout, stderr = run_command(cmd, check=False)

        results.append(
            {
                "file": compose_file,
                "valid": success,
                "message": "Valid configuration" if success else f"Invalid: {stderr}",
            }
        )

    return results


def generate_security_summary(scan_results, test_results, compose_results=None):
    """Generate security and testing summary for GitHub."""
    summary = []
    summary.append("# 🔒 Docker Security & Testing Summary")
    summary.append("")

    # Vulnerability scanning
    if scan_results.get("image_scan"):
        summary.append("## 🛡️ Vulnerability Scanning")
        summary.append(
            f"- **Image Scan**: {'✅ Completed' if scan_results['image_scan']['success'] else '❌ Failed'}"
        )
        if scan_results.get("fs_scan"):
            summary.append(
                f"- **Filesystem Scan**: {'✅ Completed' if scan_results['fs_scan']['success'] else '❌ Failed'}"
            )
        summary.append("")

    # SBOM
    if scan_results.get("sbom"):
        summary.append("## 📋 Software Bill of Materials")
        summary.append(
            f"- **SBOM Generation**: {'✅ Completed' if scan_results['sbom']['success'] else '❌ Failed'}"
        )
        summary.append("")

    # Image tests
    if test_results:
        summary.append("## 🧪 Image Testing")
        for test in test_results:
            status = "✅" if test["passed"] else "❌"
            summary.append(f"- **{test['name']}**: {status} {test['message']}")
        summary.append("")

    # Compose validation
    if compose_results:
        summary.append("## 🐳 Docker Compose Validation")
        for result in compose_results:
            status = "✅" if result["valid"] else "❌"
            summary.append(f"- **{result['file']}**: {status} {result['message']}")
        summary.append("")

    return "\n".join(summary)


def main():
    """Main entry point."""
    if len(sys.argv) < 2:
        print("Usage: docker-security.py <command> [args...]")
        print("Commands:")
        print("  scan-image <image_ref>")
        print("  scan-filesystem [path]")
        print("  generate-sbom <image_ref>")
        print("  test-image <image_ref>")
        print("  validate-compose <file1> [file2...]")
        sys.exit(1)

    command = sys.argv[1]

    try:
        if command == "scan-image":
            if len(sys.argv) < 3:
                print("Error: image reference required")
                sys.exit(1)
            image_ref = sys.argv[2]
            success, output_file = scan_image_with_trivy(image_ref)
            sys.exit(0 if success else 1)

        elif command == "scan-filesystem":
            path = sys.argv[2] if len(sys.argv) > 2 else "."
            success, output_file = scan_filesystem_with_trivy(path)
            sys.exit(0 if success else 1)

        elif command == "generate-sbom":
            if len(sys.argv) < 3:
                print("Error: image reference required")
                sys.exit(1)
            image_ref = sys.argv[2]
            success, output_file = generate_sbom(image_ref)
            sys.exit(0 if success else 1)

        elif command == "test-image":
            if len(sys.argv) < 3:
                print("Error: image reference required")
                sys.exit(1)
            image_ref = sys.argv[2]
            results = test_image_functionality(image_ref)

            # Output results as JSON for GitHub Actions
            print(json.dumps(results, indent=2))

            # Exit with error if any test failed
            failed_tests = [t for t in results if not t["passed"]]
            sys.exit(1 if failed_tests else 0)

        elif command == "validate-compose":
            if len(sys.argv) < 3:
                print("Error: compose file(s) required")
                sys.exit(1)
            compose_files = sys.argv[2:]
            results = validate_compose_files(compose_files)

            # Output results as JSON
            print(json.dumps(results, indent=2))

            # Exit with error if any validation failed
            failed_validations = [r for r in results if not r["valid"]]
            sys.exit(1 if failed_validations else 0)

        else:
            print(f"Unknown command: {command}")
            sys.exit(1)

    except Exception as e:
        print(f"Error executing {command}: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
