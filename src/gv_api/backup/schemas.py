from gv_api.core.encoding import ApiModel, Timestamp


class Backup(ApiModel):
    name: str
    size: int
    created_at: Timestamp
