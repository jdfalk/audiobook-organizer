#!/usr/bin/env python3
# file: tests/e2e/test_dashboard_workflow.py
# version: 1.1.0
# guid: 3b4c5d6e-7f89-0123-4567-89abcdef0123

"""
End-to-end tests for Dashboard workflow:
- Verify dashboard loads
- Check import folders count
- Verify library statistics display
"""

import pytest
import time
from selenium.webdriver.common.by import By


class TestDashboardWorkflow:
    """Tests for Dashboard page workflows."""
    
    def test_dashboard_loads(self, authenticated_driver):
        """Test that Dashboard page loads successfully."""
        driver = authenticated_driver
        
        # Should be on Dashboard by default or navigate to it
        try:
            dashboard_link = driver.find_element(By.XPATH, "//a[contains(text(), 'Dashboard')]")
            dashboard_link.click()
            time.sleep(1)
        except Exception:
            pass  # Already on dashboard
        
        # Verify Dashboard content
        assert "Dashboard" in driver.page_source or "Library Statistics" in driver.page_source
    
    def test_import_folders_display(self, authenticated_driver):
        """Test that import folders count is displayed on Dashboard."""
        driver = authenticated_driver
        
        # Navigate to Dashboard
        try:
            dashboard_link = driver.find_element(By.XPATH, "//a[contains(text(), 'Dashboard')]")
            dashboard_link.click()
            time.sleep(2)
        except Exception:
            pass
        
        # Look for import folders stat
        # This might be in a card or stat display
        page_source = driver.page_source.lower()
        assert "import" in page_source or "folder" in page_source, "Dashboard missing folder-related content"
        
        # Try to find specific stat element (adjust selector based on actual implementation)
        try:
            # Look for any element containing folder count
            folder_elements = driver.find_elements(By.XPATH, "//*[contains(text(), 'Import Folders') or contains(text(), 'import folders')]")
            assert len(folder_elements) > 0, "No 'Import Folders' text found on dashboard"
            print(f"Found {len(folder_elements)} elements mentioning import folders")
        except Exception as e:
            pytest.fail(f"Could not verify import folders display: {e}")
    
    def test_library_stats_display(self, authenticated_driver):
        """Test that library statistics are displayed."""
        driver = authenticated_driver
        
        # Navigate to Dashboard
        try:
            dashboard_link = driver.find_element(By.XPATH, "//a[contains(text(), 'Dashboard')]")
            dashboard_link.click()
            time.sleep(2)
        except Exception:
            pass
        
        # Check for common stat labels
        expected_stats = ["Books", "Authors", "Series"]
        page_source = driver.page_source
        
        for stat in expected_stats:
            assert stat in page_source, f"Dashboard missing '{stat}' statistic"
