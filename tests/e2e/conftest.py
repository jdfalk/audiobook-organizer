# file: tests/e2e/conftest.py
# version: 1.0.2
# guid: 3f2e1d4c-5b6a-7c8d-9e0f-1a2b3c4d5e6f

import os

import pytest

try:
    from selenium import webdriver
    from selenium.webdriver.chrome.options import Options
    from selenium.webdriver.chrome.service import Service
    from webdriver_manager.chrome import ChromeDriverManager
except ImportError:
    pytest.skip(
        "E2E browser tests require selenium/webdriver setup; skipping in CI",
        allow_module_level=True,
    )


@pytest.fixture(scope="session")
def base_url():
    return os.getenv("TEST_BASE_URL", "http://localhost:8888")


@pytest.fixture(scope="session")
def driver():
    options = Options()
    options.add_argument("--headless=new")
    options.add_argument("--disable-gpu")
    options.add_argument("--window-size=1400,900")
    options.add_argument("--no-sandbox")
    service = Service(ChromeDriverManager().install())
    drv = webdriver.Chrome(service=service, options=options)
    yield drv
    drv.quit()
