import os
from pathlib import Path
from ufoundry.config import loader

def test_legacy_env_and_home(tmp_path, monkeypatch):
    monkeypatch.setattr(Path, "home", lambda: tmp_path)
    (tmp_path / ".umabot").mkdir()
    (tmp_path / ".umabot" / "umabot.db").write_text("x")
    (tmp_path / ".umabot" / ".env").write_text("UMABOT_LLM_MODEL=legacy-model\n")
    loader._migrate_legacy_home()
    assert (tmp_path / ".ufoundry" / "ufoundry.db").read_text() == "x"
    assert (tmp_path / ".umabot").is_symlink()
    env = loader._with_legacy_names({"UMABOT_LLM_MODEL": "m", "UFOUNDRY_LLM_PROVIDER": "claude"})
    assert env["UFOUNDRY_LLM_MODEL"] == "m" and env["UFOUNDRY_LLM_PROVIDER"] == "claude"
    env = loader._with_legacy_names({"UMABOT_LLM_MODEL": "old", "UFOUNDRY_LLM_MODEL": "new"})
    assert env["UFOUNDRY_LLM_MODEL"] == "new"
