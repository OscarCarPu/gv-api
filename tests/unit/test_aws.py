"""boto3 clients take the deploy's AWS settings."""

import pytest

from gv_api.core import aws


def test_region_and_fake_endpoint(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("AWS_REGION", "eu-south-2")
    monkeypatch.setenv("AWS_ENDPOINT_URL", "http://aws:5000")
    monkeypatch.setenv("AWS_ACCESS_KEY_ID", "test")
    monkeypatch.setenv("AWS_SECRET_ACCESS_KEY", "test")
    s3 = aws.client("s3")
    assert s3.meta.region_name == "eu-south-2"
    assert s3.meta.endpoint_url == "http://aws:5000"
    assert s3.meta.config.s3["addressing_style"] == "path"


def test_real_aws_uses_virtual_hosts(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("AWS_REGION", "eu-south-2")
    monkeypatch.delenv("AWS_ENDPOINT_URL", raising=False)
    monkeypatch.setenv("AWS_ACCESS_KEY_ID", "test")
    monkeypatch.setenv("AWS_SECRET_ACCESS_KEY", "test")
    ssm = aws.client("ssm")
    assert ssm.meta.region_name == "eu-south-2"
    assert not (ssm.meta.config.s3 or {}).get("addressing_style")
