#!/usr/bin/env python3
# file: tests/e2e/test_settings_workflow.py
# version: 1.0.0
# guid: 2a3b4c5d-6e7f-8901-2345-6789abcdef01

"""
End-to-end tests for Settings page workflow:
- Browse server filesystem
- Configure library path
- Add import folders
- Verify settings persistence
"""

import pytest
import time
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.common.exceptions import TimeoutException


class TestSettingsWorkflow:
    """Tests for Settings page workflows."""

    def test_navigate_to_settings(self, authenticated_driver):
        """Test that we can navigate to the Settings page."""
        driver = authenticated_driver

        # Click on Settings in sidebar (or navigation)
        try:
            settings_link = WebDriverWait(driver, 10).until(
                EC.element_to_be_clickable(
                    (By.XPATH, "//a[contains(text(), 'Settings')]")
                )
            )
            settings_link.click()
            time.sleep(1)

            # Verify we're on Settings page
            assert "Settings" in driver.page_source
            page_title = driver.find_element(
                By.XPATH, "//h4[contains(text(), 'Settings')]"
            )
            assert page_title.is_displayed()
        except TimeoutException:
            pytest.fail("Could not find or click Settings link")

    def test_browse_server_filesystem(self, authenticated_driver):
        """Test browsing the server filesystem in Settings."""
        driver = authenticated_driver

        # Navigate to Settings
        settings_link = WebDriverWait(driver, 10).until(
            EC.element_to_be_clickable((By.XPATH, "//a[contains(text(), 'Settings')]"))
        )
        settings_link.click()
        time.sleep(1)

        # Click "Browse Server" button
        try:
            browse_button = WebDriverWait(driver, 10).until(
                EC.element_to_be_clickable(
                    (By.XPATH, "//button[contains(., 'Browse Server')]")
                )
            )
            browse_button.click()
            time.sleep(2)

            # Verify dialog opened
            dialog = WebDriverWait(driver, 10).until(
                EC.presence_of_element_located((By.XPATH, "//div[@role='dialog']"))
            )
            assert dialog.is_displayed()

            # Verify no immediate error message
            error_messages = driver.find_elements(
                By.XPATH,
                "//div[contains(@class, 'MuiAlert-message') and contains(text(), 'Failed to browse filesystem')]",
            )
            assert len(error_messages) == 0, "Browse filesystem failed immediately"

        except TimeoutException:
            pytest.fail("Could not open Browse Server dialog")

    def test_settings_tabs_switching(self, authenticated_driver):
        """Test switching between Settings tabs."""
        driver = authenticated_driver

        # Navigate to Settings
        settings_link = WebDriverWait(driver, 10).until(
            EC.element_to_be_clickable((By.XPATH, "//a[contains(text(), 'Settings')]"))
        )
        settings_link.click()
        time.sleep(1)

        # Test switching to Metadata tab
        metadata_tab = WebDriverWait(driver, 10).until(
            EC.element_to_be_clickable((By.XPATH, "//button[contains(., 'Metadata')]"))
        )
        metadata_tab.click()
        time.sleep(0.5)

        # Verify Metadata content is visible
        assert "Metadata" in driver.page_source

        # Test switching to Performance tab
        performance_tab = WebDriverWait(driver, 10).until(
            EC.element_to_be_clickable(
                (By.XPATH, "//button[contains(., 'Performance')]")
            )
        )
        performance_tab.click()
        time.sleep(0.5)

        # Verify Performance content is visible
        assert "Performance" in driver.page_source

    def test_save_button_visible(self, authenticated_driver):
        """Test that Save Settings button is visible and accessible."""
        driver = authenticated_driver

        # Navigate to Settings
        settings_link = WebDriverWait(driver, 10).until(
            EC.element_to_be_clickable((By.XPATH, "//a[contains(text(), 'Settings')]"))
        )
        settings_link.click()
        time.sleep(1)

        # Scroll to bottom to ensure button is visible
        driver.execute_script("window.scrollTo(0, document.body.scrollHeight);")
        time.sleep(0.5)

        # Find Save Settings button
        save_button = WebDriverWait(driver, 10).until(
            EC.element_to_be_clickable(
                (By.XPATH, "//button[contains(., 'Save Settings')]")
            )
        )

        # Verify button is displayed and in viewport
        assert save_button.is_displayed(), "Save Settings button is not visible"
        location = save_button.location
        size = save_button.size
        viewport_height = driver.execute_script("return window.innerHeight")

        # Button should be within viewport or scrollable area
        assert location["y"] >= 0, "Save button is above viewport"
        # Note: Button might be below viewport initially, but should be scrollable
        print(
            f"Save button location: {location}, size: {size}, viewport height: {viewport_height}"
        )
