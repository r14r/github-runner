"""BookBuilder runner images must provide the exact host tools required by CI."""
from pathlib import Path
import os
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def repo_profile(repo, **overrides):
    env = dict(os.environ)
    env.pop("RUNNER_PROFILE", None)
    env.pop("RUNNER_IMAGE", None)
    env.update(overrides)
    script = (
        'source runner/lib/common.sh; set_repo "$1"; '
        'printf "%s|%s|%s" "$RUNNER_PROFILE" "$RUNNER_IMAGE" "$RUNNER_VOLUME"'
    )
    return subprocess.check_output(
        ["bash", "-c", script, "_", repo],
        cwd=ROOT, env=env, text=True,
    ).strip()


def test_bookbuilder_profile_is_selected_automatically():
    assert repo_profile("r14r/BookBuilder") == (
        "bookbuilder|github-actions-runner-local:2.338.1-bookbuilder|"
        "github-runner-r14r-bookbuilder-state"
    )


def test_other_repositories_keep_generic_image():
    assert repo_profile("r14r/github-runner-test") == (
        "runner-base|github-actions-runner-local:2.338.1|"
        "github-runner-r14r-github-runner-test-state"
    )


def test_explicit_profile_and_image_override():
    assert repo_profile(
        "example/project", RUNNER_PROFILE="bookbuilder", RUNNER_IMAGE="custom:test"
    ).startswith("bookbuilder|custom:test|")


def test_dockerfile_builds_verified_bookbuilder_profile():
    dockerfile = (ROOT / "runner/docker/Dockerfile").read_text()
    assert "FROM ubuntu:24.04 AS runner-base" in dockerfile
    assert "FROM runner-base AS bookbuilder" in dockerfile
    for required in (
        "ffmpeg", "fontconfig", "openjdk-21-jre-headless",
        "php8.2-cli", "php8.2-curl", "php8.2-mbstring", "php8.2-zip",
        "JUST_VERSION=1.58.0", "TYPST_VERSION=0.15.1",
        "sha256sum -c -",
    ):
        assert required in dockerfile
    assert "AllowInsecureRepositories" not in dockerfile
    assert "trusted=yes" not in dockerfile


def test_download_runs_original_doctor_in_profile_image():
    download = (ROOT / "runner/01-download.sh").read_text()
    assert '--target "$RUNNER_PROFILE"' in download
    assert '--entrypoint bash "$RUNNER_IMAGE" -s' in download
    assert 'doctor-linux.sh' in download
    assert 'docker volume create "$RUNNER_VOLUME"' in download


def test_container_replacement_is_explicit_and_keeps_registration_volume():
    service = (ROOT / "runner/04-run.sh").read_text()
    assert "replace-container)" in service
    assert "docker rm \"$RUNNER_CONTAINER\"" in service
    assert 'docker rm -v' not in service
    assert "State.Running" in service
    assert "RUNNER_DOCKER_SOCKET" in service
