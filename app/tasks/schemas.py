from pydantic import BaseModel


class ProjectBase(BaseModel):
    name: str
    description: str | None = None
    finished: bool = False
