"""The AWS side the failover reads: the state in SSM and the dumps in S3 (boto3, in threads)."""

import asyncio
from pathlib import Path
from typing import TYPE_CHECKING

from gv_api.core import aws

if TYPE_CHECKING:
    from mypy_boto3_s3 import S3Client
    from mypy_boto3_ssm import SSMClient


class NoParamError(Exception):
    pass


class SSMParams:
    def __init__(self) -> None:
        self._client: SSMClient | None = None

    async def get(self, name: str) -> str:
        return await asyncio.to_thread(self._get, name)

    def _get(self, name: str) -> str:
        if self._client is None:
            self._client = aws.client("ssm")
        try:
            out = self._client.get_parameter(Name=name)
        except self._client.exceptions.ParameterNotFound as err:
            raise NoParamError(name) from err
        return out["Parameter"].get("Value", "")


class S3Store:
    def __init__(self, bucket: str) -> None:
        self._bucket = bucket
        self._client: S3Client | None = None

    async def download(self, key: str, dest: Path) -> None:
        await asyncio.to_thread(self._download, key, dest)

    def _download(self, key: str, dest: Path) -> None:
        """Through a temporary file, so dest is never half written."""
        if self._client is None:
            self._client = aws.client("s3")
        tmp = dest.with_name(dest.name + ".part")
        try:
            self._client.download_file(self._bucket, key, str(tmp))
            tmp.rename(dest)
        finally:
            tmp.unlink(missing_ok=True)
