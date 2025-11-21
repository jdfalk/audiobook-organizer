#!/usr/bin/env python3
# file: tests/e2e/conftest.py
# version: 1.0.0
# guid: 1f2e3d4c-5b6a-7980-abcd-ef1234567890

"""
Pytest configuration and fixtures for Selenium end-to-end tests.
"""

import os
import pytest
import time
from selenium import webdriver
from selenium.webdriver.chrome.service import Service
from selenium.webdriver.chrome.options import Options
from webdriver_manager.chrome import ChromeDriverManager


@pytest.fixture(scope="session")
def base_url():
    """Base URL for the application under test."""
    return os.getenv("TEST_BASE_URL", "http://localhost:8080")


@pytest.fixture(scope="function")
def driver(base_url):
    """
    Create and configure Selenium WebDriver for Chrome.
    This fixture is function-scoped so each test gets a fresh browser.
    """
    chrome_options = Options()
    chrome_options.add_argument("--headless")  # Run in headless mode
    chrome_options.add_argument("--no-sandbox")
    chrome_options.add_argument("--disable-dev-shm-usage")
    chrome_options.add_argument("--window-size=1920,1080")
    
    # Create WebDriver
    service = Service(ChromeDriverManager().install())
    driver = webdriver.Chrome(service=service, options=chrome_options)
    driver.implicitly_wait(10)  # Wait up to 10 seconds for elements
    
    yield driver
    
    # Cleanup
    driver.quit()


@pytest.fixture(scope="function")
def authenticated_driver(driver, base_url):
    """
    WebDriver that is already authenticated (if auth is required).
    For now, just navigates to base URL.
    """
    driver.get(base_url)
    time.sleep(1)  # Wait for page load
    return driver
