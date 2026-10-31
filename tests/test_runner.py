"""Regression checks for the standalone GitHub Actions runner manager."""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

def test_provisioning_includes_runner_label_and_unprivileged_service():
    installer = (ROOT / "install-ubuntu.sh").read_text()
    assert '--labels "$5"' in installer
    assert 'svc.sh install "$2"' in installer
    assert "--token" in installer
    assert "sha256sum -c" in installer

    mac = (ROOT / "install-macos.sh").read_text()
    assert '--labels "${RUNNER_LABEL:-bookbuilder}"' in mac
    assert "shasum -a 256 -c -" in mac
    assert "gh api --method POST" in mac
    assert "./svc.sh install" in mac
    assert "sudo ./svc.sh" not in mac
    assert 'Darwin)' in (ROOT / "doctor.sh").read_text()



def test_runner_shell_scripts_parse():
    import subprocess
    scripts = list((ROOT).glob("*.sh"))
    assert len(scripts) >= 5
    for script in scripts:
        subprocess.run(["bash", "-n", str(script)], check=True)



def test_preflights_use_php_extension_api_and_accept_typst_commit_suffix():
    """Avoid SIGPIPE with grep -q under pipefail and overly strict Typst text equality."""
    for name in ("doctor-linux.sh", "doctor-macos.sh"):
        script = (ROOT / name).read_text()
        assert "extension_loaded($argv[1])" in script
        assert "php -m | grep" not in script
        assert "0[.]15[.]1([[:space:]]|$)" in script
    for name in ("install-ubuntu.sh", "install-macos.sh"):
        installer = (ROOT / name).read_text()
        assert "0[.]15[.]1([[:space:]]|$)" in installer



def test_macos_doctor_accepts_builtin_extensions_and_typst_commit_suffix(tmp_path):
    """Emulate a correct Intel Mac in a Linux CI process without provisioning macOS."""
    import os
    import subprocess

    bindir = tmp_path / "bin"
    bindir.mkdir()
    mocks = {
        "uname": '#!/bin/sh\ncase "$1" in -s) echo Darwin ;; -m) echo x86_64 ;; esac\n',
        "brew": '#!/bin/sh\ncase "$1" in --prefix) printf "/mock/brew";; esac\n',
        "python3.12": '#!/bin/sh\nprintf "OK: Python 3.12 venv, pip and SSL\\n"\n',
        "php": '''#!/bin/sh
case "$2" in
  *PHP_MAJOR_VERSION*) printf "8.2" ;;
  *extension_loaded*) case "$3" in curl|zip|mbstring) exit 0 ;; esac; exit 1 ;;
  *) exit 1 ;;
esac
''',
        "typst": '#!/bin/sh\nprintf "typst 0.15.1 (9dfd3a0)\\n"\n',
        "java": '#!/bin/sh\nprintf \'openjdk version "21.0.13"\\n\' >&2\n',
    }
    for name in ("git", "curl", "unzip", "zip", "jq", "tar", "xz", "just",
                 "ffmpeg", "fc-list", "gh"):
        mocks[name] = "#!/bin/sh\nexit 0\n"
    for name, body in mocks.items():
        dest = bindir / name
        dest.write_text(body)
        dest.chmod(0o755)
    # Keep an existing developer's ~/.local/bin/typst out of the emulated PATH.
    env = dict(os.environ, PATH=f"{bindir}:{os.environ['PATH']}", HOME=str(tmp_path))
    result = subprocess.run(
        ["bash", str(ROOT / "doctor-macos.sh")],
        env=env, capture_output=True, text=True, check=False,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert "PHP extension curl" in result.stdout
    assert "PHP extension mbstring" in result.stdout
    assert "Typst 0.15.1" in result.stdout



def test_standalone_justfile_has_six_working_commands():
    recipe = (ROOT / "justfile").read_text()
    for command in ("install", "doctor", "start", "stop", "status"):
        assert f"\n{command}:" in recipe
    assert "\nmonitor interval=" in recipe
    assert "install -m 0755 dist/github-runner-cli" in recipe
    assert "go build" in recipe
    for action in ("start", "stop", "status", "monitor"):
        assert f"bash ./control.sh {action}" in recipe
    assert not (ROOT / "obsolete-scripts-runner").exists()
    assert not (ROOT / "obsolete-docs-runner").exists()



def test_monitor_and_start_stop_status_without_real_services(tmp_path):
    """Test the lifecycle dispatch and one monitor refresh without installing GitHub."""
    import os
    import subprocess

    runner = tmp_path / "runner"
    runner.mkdir()
    log = tmp_path / "commands.log"
    svc = runner / "svc.sh"
    svc.write_text('#!/bin/sh\nprintf "%s\\n" "$1" >> "$RUNNER_SERVICE_TEST_LOG"\n')
    svc.chmod(0o755)
    (runner / "_diag").mkdir()
    (runner / "_diag" / "Runner_20261025-123456.log").write_text("Diagnostics smoke line\n")
    mocks = tmp_path / "mocks"
    mocks.mkdir()
    uname = mocks / "uname"
    uname.write_text('#!/bin/sh\nprintf "Darwin\\n"\n')
    uname.chmod(0o755)
    gh = mocks / "gh"
    gh.write_text('#!/bin/sh\necho "recent workflow mock"\n')
    gh.chmod(0o755)
    env = dict(os.environ, RUNNER_DIR=str(runner), RUNNER_SERVICE_TEST_LOG=str(log),
               RUNNER_MONITOR_ONCE="1", PATH=f"{mocks}:{os.environ['PATH']}")
    controller = str(ROOT / "control.sh")
    for command in ("status", "start", "stop"):
        subprocess.run(["bash", controller, command], check=True, env=env, capture_output=True)
    monitored = subprocess.run(
        ["bash", controller, "monitor", "2"], check=True, env=env,
        capture_output=True, text=True,
    )
    assert "recent workflow mock" in monitored.stdout
    assert "Diagnostics smoke line" in monitored.stdout
    assert log.read_text().splitlines() == ["status", "start", "stop", "status"]



def test_monitor_rejects_invalid_interval(tmp_path):
    import os
    import subprocess

    ctl = ROOT / "control.sh"
    for arg in ("x", "0", "1"):
        result = subprocess.run(["bash", str(ctl), "monitor", arg],
                                env=os.environ, capture_output=True, text=True)
        assert result.returncode == 2



def test_macos_runner_runtime_is_gitignored_and_repository_local():
    root = ROOT
    ignore = (ROOT / ".gitignore").read_text()
    installer = (root / "install-macos.sh").read_text()
    controller = (root / "control.sh").read_text()
    migrator = (root / "migrate-macos.sh").read_text()
    recipes = (root / "justfile").read_text()

    assert ".local/" in ignore
    for script in (installer, controller, migrator):
        assert ".local/bookbuilder-actions-runner" in script
    assert 'bash "$SCRIPT_DIR/migrate-macos.sh"' in installer
    assert "\nmigrate:" in recipes
    assert "bash ./migrate-macos.sh" in recipes
    assert "gh api" not in migrator
    assert "config.sh --" not in migrator



def _fake_macos_migration(tmp_path):
    import os

    old = tmp_path / "home-old-runner"
    old.mkdir()
    (old / "config.sh").write_text("# existing runner\n")
    (old / ".runner").write_text('{"agentId": 123}\n')
    (old / ".credentials").write_text('{"protected":"existing"}\n')
    (old / ".service").write_text(str(tmp_path / "old-launchd.plist") + "\n")
    (old / "_work").mkdir()
    (old / "_work" / "artifact.txt").write_text("preserve artifact")
    log = tmp_path / "events.log"
    (old / "svc.sh").write_text(
        '#!/bin/sh\n'
        'echo "$PWD:$1" >> "$RUNNER_SERVICE_TEST_LOG"\n'
        'case "$1" in\n'
        '  uninstall) rm -f .service ;;\n'
        '  install) printf "%s/new-service.plist\\n" "$PWD" > .service ;;\n'
        'esac\n'
    )
    (old / "svc.sh").chmod(0o755)
    mocks = tmp_path / "mocks"
    mocks.mkdir()
    for name, content in {
        "uname": '#!/bin/sh\nprintf "Darwin\\n"\n',
        "id": '#!/bin/sh\nprintf "501\\n"\n',
    }.items():
        program = mocks / name
        program.write_text(content)
        program.chmod(0o755)
    target = tmp_path / "repo" / "github-runner" / ".local" / "bookbuilder-actions-runner"
    env = dict(
        os.environ,
        LEGACY_RUNNER_DIR=str(old),
        RUNNER_DIR=str(target),
        RUNNER_SERVICE_TEST_LOG=str(log),
        PATH=f"{mocks}:{os.environ['PATH']}",
    )
    return old, target, log, env



def test_macos_migration_preserves_registration_and_reinstalls_launchd(tmp_path):
    import subprocess

    old, target, log, env = _fake_macos_migration(tmp_path)
    script = ROOT / "migrate-macos.sh"
    process = subprocess.run(
        ["bash", str(script)], env=env, capture_output=True, text=True
    )
    assert process.returncode == 0, process.stdout + process.stderr
    assert not old.exists()
    assert (target / ".runner").read_text() == '{"agentId": 123}\n'
    assert (target / ".credentials").read_text() == '{"protected":"existing"}\n'
    assert (target / "_work" / "artifact.txt").read_text() == "preserve artifact"
    assert str(target) in (target / ".service").read_text()
    assert log.read_text().splitlines() == [
        f"{old}:stop", f"{old}:uninstall",
        f"{target}:install", f"{target}:start", f"{target}:status",
    ]
    # Idempotent: the original location is gone and service is not disturbed.
    again = subprocess.run(["bash", str(script)], env=env, capture_output=True, text=True)
    assert again.returncode == 0, again.stderr
    assert len(log.read_text().splitlines()) == 5



def test_macos_migration_refuses_overwrite_before_service_stop(tmp_path):
    import subprocess

    old, target, log, env = _fake_macos_migration(tmp_path)
    target.mkdir(parents=True)
    (target / ".credentials").write_text("another runner")
    process = subprocess.run(
        ["bash", str(ROOT / "migrate-macos.sh")],
        env=env, capture_output=True, text=True
    )
    assert process.returncode != 0
    assert "Both legacy and target" in process.stderr
    assert (old / ".credentials").exists()
    assert (target / ".credentials").read_text() == "another runner"
    assert not log.exists()



def test_macos_migration_tolerates_old_empty_installer_directory(tmp_path):
    import subprocess

    old, target, log, env = _fake_macos_migration(tmp_path)
    for child in old.iterdir():
        if child.is_dir():
            for item in child.iterdir():
                item.unlink()
            child.rmdir()
        else:
            child.unlink()
    result = subprocess.run(
        ["bash", str(ROOT / "migrate-macos.sh")],
        env=env, capture_output=True, text=True
    )
    assert result.returncode == 0, result.stderr
    assert not old.exists()
    assert not target.exists()
    assert not log.exists()



def test_repair_is_explicit_and_preserves_runner_workspaces():
    runner = ROOT
    just = (runner / "justfile").read_text()
    repair = (runner / "repair-macos.sh").read_text()
    monitor = (runner / "control.sh").read_text()

    assert "\nrepair:\n    bash ./repair-macos.sh" in just
    assert 'bash ./config.sh remove --local' in repair
    assert "registration-token" in repair
    assert "--replace" in repair
    assert '[[ "$busy" != "true" ]]' in repair
    assert '[[ "$state" != "online" ]]' in repair
    assert "rm -rf" not in repair
    assert "gh api" in monitor
    assert "The runner registration has been deleted from the server" in monitor
    assert "status=$state" in monitor



def _mock_macos_repair(tmp_path, github_state):
    import os

    runner = tmp_path / "runner"
    runner.mkdir()
    (runner / ".runner").write_text('{"agentName": "server-bookbuilder-macos"}\n')
    (runner / ".credentials").write_text('{"token":"stale"}\n')
    (runner / ".service").write_text("old-launchd-service\n")
    (runner / "_work").mkdir()
    (runner / "_work" / "keep.txt").write_text("important build output")
    (runner / "_diag").mkdir()
    (runner / "_diag" / "Runner_old.log").write_text("old errors")
    operations = tmp_path / "ops.log"
    (runner / "svc.sh").write_text(
        '#!/bin/sh\n'
        'echo "service:$1" >> "$REPAIR_TEST_OPS"\n'
        'case "$1" in\n'
        '  uninstall) rm -f .service ;;\n'
        '  install) echo new-service > .service ;;\n'
        'esac\n'
    )
    (runner / "config.sh").write_text(
        '#!/bin/sh\n'
        'echo "config:$1:$2" >> "$REPAIR_TEST_OPS"\n'
        'case "$1" in\n'
        '  remove) rm -f .runner .credentials ;;\n'
        '  --unattended) echo \'{"agentName":"server-bookbuilder-macos"}\' > .runner;'
        ' echo \'{"token":"new"}\' > .credentials ;;\n'
        'esac\n'
    )
    (runner / "config.sh").chmod(0o755)
    (runner / "svc.sh").chmod(0o755)
    mocks = tmp_path / "mocks"
    mocks.mkdir()
    for name, source in {
        "uname": '#!/bin/sh\necho Darwin\n',
        "id": '#!/bin/sh\necho 501\n',
        "jq": '''#!/bin/sh
case "$*" in
  *agentName*) echo server-bookbuilder-macos ;;
  *busy*) echo false ;;
  *status*) echo __STATUS__ ;;
esac
'''.replace("__STATUS__", github_state),
        "gh": '''#!/bin/sh
if [ "$1" = "auth" ]; then exit 0; fi
case "$*" in
  *registration-token*) echo repair_token ;;
  *) echo '{"runners":[{"name":"server-bookbuilder-macos","status":"offline","busy":false}]}' ;;
esac
''',
    }.items():
        executable = mocks / name
        executable.write_text(source)
        executable.chmod(0o755)
    env = dict(os.environ, RUNNER_DIR=str(runner), REPAIR_TEST_OPS=str(operations),
               PATH=f"{mocks}:{os.environ['PATH']}")
    return runner, operations, env



def test_repair_macos_reconfigures_offline_runner_without_erasing_work(tmp_path):
    import subprocess

    runner, operations, env = _mock_macos_repair(tmp_path, "offline")
    repaired = subprocess.run(
        ["bash", str(ROOT / "repair-macos.sh")],
        env=env, capture_output=True, text=True
    )
    assert repaired.returncode == 0, repaired.stdout + repaired.stderr
    assert (runner / ".credentials").read_text() == '{"token":"new"}\n'
    assert (runner / "_work" / "keep.txt").read_text() == "important build output"
    assert (runner / "_diag" / "Runner_old.log").read_text() == "old errors"
    assert operations.read_text().splitlines() == [
        "service:stop",
        "service:uninstall",
        "config:remove:--local",
        "config:--unattended:--url",
        "service:install",
        "service:start",
        "service:status",
    ]



def test_repair_macos_refuses_online_runner(tmp_path):
    import subprocess

    runner, operations, env = _mock_macos_repair(tmp_path, "online")
    outcome = subprocess.run(
        ["bash", str(ROOT / "repair-macos.sh")],
        env=env, capture_output=True, text=True
    )
    assert outcome.returncode != 0
    assert "ONLINE" in outcome.stderr
    assert (runner / ".credentials").read_text() == '{"token":"stale"}\n'
    assert not operations.exists()



def _fake_actions_gh(tmp_path, before, after=None):
    """Mock GitHub CLI for safe, off-network Actions cleanup integration tests."""
    import os

    tmp_path.mkdir(parents=True, exist_ok=True)
    mocks = tmp_path / "mocks"
    mocks.mkdir()
    script = mocks / "gh"
    script.write_text('''#!/usr/bin/env bash
case "$1" in
  auth) exit 0 ;;
  api)
    if [[ -f "$MOCK_CANCELED" ]]; then
      cat "$MOCK_AFTER"
    else
      cat "$MOCK_BEFORE"
    fi
    ;;
  run)
    echo "$2 $3" >> "$MOCK_OPERATIONS"
    if [[ "$2" == "cancel" ]]; then touch "$MOCK_CANCELED"; fi
    ;;
  *) exit 90 ;;
esac
''')
    script.chmod(0o755)
    initial, later = tmp_path / "before.tsv", tmp_path / "after.tsv"
    initial.write_text(before)
    later.write_text(after if after is not None else before)
    operations = tmp_path / "operations.log"
    env = dict(
        os.environ,
        MOCK_CANCELED=str(tmp_path / "canceled"),
        MOCK_BEFORE=str(initial),
        MOCK_AFTER=str(later),
        MOCK_OPERATIONS=str(operations),
        ACTIONS_MAX_POLLS="2",
        ACTIONS_POLL_SECONDS="0",
        PATH=f"{mocks}:{os.environ['PATH']}",
    )
    return env, operations



def _run_actions_cleanup(command, env, confirmation=""):
    import subprocess

    return subprocess.run(
        ["bash", str(ROOT / "actions.sh"), command],
        env=env, input=confirmation, text=True, capture_output=True
    )



def test_actions_cleanup_just_recipes_and_shell_parse():
    recipes = (ROOT / "justfile").read_text()
    for command in ("cancel", "delete", "clear"):
        assert f"\nactions-{command}:" in recipes
        assert f"bash ./actions.sh {command}" in recipes
    actions = (ROOT / "actions.sh").read_text()
    assert "gh api --paginate" in actions
    assert "confirm_delete" in actions
    assert "await_completion" in actions
    assert "DELETE $REPO" in actions
    assert "gh run cancel" in actions
    assert "gh run delete" in actions
    assert "rm -rf \"$work\"" in actions  # removes only the temporary snapshot



def test_actions_cancel_only_active_runs_without_deleting(tmp_path):
    env, operations = _fake_actions_gh(
        tmp_path, "101\tqueued\n102\tin_progress\n103\tcompleted\n"
    )
    result = _run_actions_cleanup("cancel", env)
    assert result.returncode == 0, result.stdout + result.stderr
    assert operations.read_text().splitlines() == ["cancel 101", "cancel 102"]



def test_actions_delete_requires_exact_repository_confirmation(tmp_path):
    env, operations = _fake_actions_gh(
        tmp_path, "101\tqueued\n102\tcompleted\n103\tcompleted\n"
    )
    no = _run_actions_cleanup("delete", env, "DELETE\n")
    assert no.returncode != 0
    assert not operations.exists()
    yes = _run_actions_cleanup(
        "delete", env, "DELETE r14r/BookBuilder-with-typst\n"
    )
    assert yes.returncode == 0, yes.stdout + yes.stderr
    assert operations.read_text().splitlines() == ["delete 102", "delete 103"]



def test_actions_clear_deletes_only_initial_runs_after_cancel(tmp_path):
    env, operations = _fake_actions_gh(
        tmp_path,
        "101\tqueued\n102\tin_progress\n103\tcompleted\n",
        "101\tcompleted\n102\tcompleted\n103\tcompleted\n104\tcompleted\n",
    )
    completed = _run_actions_cleanup(
        "clear", env, "DELETE r14r/BookBuilder-with-typst\n"
    )
    assert completed.returncode == 0, completed.stdout + completed.stderr
    assert operations.read_text().splitlines() == [
        "cancel 101", "cancel 102",
        "delete 101", "delete 102", "delete 103"
    ]



def test_actions_clear_fails_closed_if_cancellation_still_pending(tmp_path):
    env, operations = _fake_actions_gh(
        tmp_path, "101\tqueued\n102\tcompleted\n"
    )
    result = _run_actions_cleanup(
        "clear", env, "DELETE r14r/BookBuilder-with-typst\n"
    )
    assert result.returncode != 0
    assert operations.read_text().splitlines() == ["cancel 101"]
    assert "No deletion attempted" in result.stderr



def test_actions_clear_without_confirmation_does_not_cancel(tmp_path):
    env, operations = _fake_actions_gh(
        tmp_path, "101\tqueued\n102\tcompleted\n"
    )
    result = _run_actions_cleanup("clear", env, "\n")
    assert result.returncode != 0
    assert not operations.exists()



def test_runner_cleanup_recipes_and_scripts():
    runner = ROOT
    just = (runner / "justfile").read_text()
    shell = (runner / "cleanup.sh").read_text()
    assert '\ncleanup days="30":' in just
    assert '\ncleanup-preview days="30":' in just
    assert 'RUNNER_CLEANUP_DRY_RUN=1' in just
    assert "find \"$diag\" -maxdepth 1 -type f -name '*.log'" in shell
    assert "RUNNER_DIR" in shell
    assert "rm -rf" not in shell
    assert ".credentials" not in shell or "Never change .runner" in shell



def _invoke_runner_cleanup(tmp_path, days, preview):
    import os
    import subprocess

    root = tmp_path / "runtime"
    diag = root / "_diag"
    diag.mkdir(parents=True)
    work = root / "_work"
    work.mkdir()
    (work / "important.txt").write_text("preserve work")
    (root / ".runner").write_text("important identity")
    (root / ".credentials").write_text("important token")
    old = diag / "Runner_old.log"
    recent = diag / "Runner_new.log"
    other = diag / "other.txt"
    old.write_text("old logs")
    recent.write_text("recent logs")
    other.write_text("old other files preserved")
    import time
    now = time.time()
    os.utime(old, (now - 35 * 86400, now - 35 * 86400))
    os.utime(other, (now - 35 * 86400, now - 35 * 86400))
    env = dict(os.environ, RUNNER_DIR=str(root),
               RUNNER_CLEANUP_DRY_RUN="1" if preview else "0")
    command = ["bash", str(ROOT / "cleanup.sh"), str(days)]
    result = subprocess.run(command, env=env, capture_output=True, text=True)
    return root, old, recent, other, result



def test_runner_cleanup_dry_run_preserves_all(tmp_path):
    root, old, recent, other, result = _invoke_runner_cleanup(tmp_path, 30, True)
    assert result.returncode == 0, result.stdout + result.stderr
    assert "DRY RUN" in result.stdout
    assert old.exists() and recent.exists() and other.exists()
    assert (root / ".credentials").exists()
    assert (root / "_work" / "important.txt").exists()



def test_runner_cleanup_removes_only_expired_regular_logs(tmp_path):
    root, old, recent, other, result = _invoke_runner_cleanup(tmp_path, 30, False)
    assert result.returncode == 0, result.stdout + result.stderr
    assert not old.exists()
    assert recent.exists() and other.exists()
    assert (root / ".runner").read_text() == "important identity"
    assert (root / ".credentials").read_text() == "important token"
    assert (root / "_work" / "important.txt").read_text() == "preserve work"



def test_runner_cleanup_rejects_invalid_retention(tmp_path):
    import os
    import subprocess

    env = dict(os.environ, RUNNER_DIR=str(tmp_path))
    script = str(ROOT / "cleanup.sh")
    for arg in ("0", "-1", "abc", "3651"):
        result = subprocess.run(["bash", script, arg],
                                env=env, capture_output=True, text=True)
        assert result.returncode != 0



def test_runner_cleanup_does_not_follow_diag_symlink(tmp_path):
    import os
    import subprocess

    root = tmp_path / "runner"
    root.mkdir()
    outside = tmp_path / "outside"
    outside.mkdir()
    sensitive = outside / "Runner_sensitive.log"
    sensitive.write_text("must not delete")
    (root / "_diag").symlink_to(outside, target_is_directory=True)
    env = dict(os.environ, RUNNER_DIR=str(root))
    result = subprocess.run(
        ["bash", str(ROOT / "cleanup.sh"), "30"],
        env=env, capture_output=True, text=True,
    )
    assert result.returncode != 0
    assert sensitive.read_text() == "must not delete"
