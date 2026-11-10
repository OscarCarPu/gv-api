from tests.e2e.conftest import APIClient, password, totp_code


def test_valid_totp_code_returns_full_token(client: APIClient) -> None:
    tmp = client.login(password())
    assert client.login_2fa(tmp, totp_code())


def test_wrong_totp_code_returns_401(client: APIClient) -> None:
    tmp = client.login(password())
    resp = client.do("POST", "/login/2fa", {"token": tmp, "code": "000000"})
    assert resp.status_code == 401


def test_authed_fixture_holds_a_full_token(authed: APIClient) -> None:
    assert authed.token.count(".") == 2
