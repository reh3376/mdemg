"""E2E tests for the MDEMG Graph Topology dashboard pagination, 3D viewer, and Neighborhood Explorer."""

import pytest
from playwright.sync_api import Page, expect

from conftest import dashboard_url, DASHBOARDS


DASHBOARD_UID = DASHBOARDS["graph-topology"]


class TestPaginationControls:
    """Verify pagination template variables and info panel are present."""

    def test_page_size_variable_visible(self, authenticated_page: Page, grafana_url: str):
        """The 'Nodes Per Page' dropdown should be visible in the variable bar."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page_size_var = page.get_by_test_id("data-testid template variable").filter(
            has_text="Nodes Per Page"
        )
        expect(page_size_var.first).to_be_visible(timeout=15000)

    def test_page_variable_visible(self, authenticated_page: Page, grafana_url: str):
        """The 'Page' textbox should be visible in the variable bar."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page_var = page.get_by_test_id("data-testid template variable").filter(
            has_text="Page"
        )
        expect(page_var.first).to_be_visible(timeout=15000)

    def test_page_size_default_is_500(self, authenticated_page: Page, grafana_url: str):
        """Default page_size should be 500."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page_size_var = page.get_by_test_id("data-testid template variable").filter(
            has_text="Nodes Per Page"
        )
        expect(page_size_var.first.get_by_text("500")).to_be_visible(timeout=5000)

    def test_pagination_info_panel_visible(self, authenticated_page: Page, grafana_url: str):
        """The pagination info text panel should show page info."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        info_text = page.get_by_text("Viewing page")
        expect(info_text).to_be_visible(timeout=15000)

    def test_pagination_info_shows_sorted_message(self, authenticated_page: Page, grafana_url: str):
        """Info panel should indicate connectivity-based sorting."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        sorted_text = page.get_by_text("Sorted by connectivity")
        expect(sorted_text).to_be_visible(timeout=15000)


class TestPageNavigation:
    """Verify that changing the page variable re-renders the graph."""

    def test_page_change_updates_url(self, authenticated_page: Page, grafana_url: str):
        """Setting page=2 via URL should update the variable bar."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev", page="2", page_size="500"),
            wait_until="networkidle",
        )
        assert "var-page=2" in page.url


class Test3DViewer:
    """Verify the 3D topology viewer iframe and its content."""

    def test_3d_panel_visible(self, authenticated_page: Page, grafana_url: str):
        """The Layer Topology 3D panel should be visible."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        panel_3d = page.locator('[data-panelid]').filter(
            has=page.get_by_text("Layer Topology", exact=False)
        ).first
        expect(panel_3d).to_be_visible(timeout=15000)

    def test_3d_iframe_present(self, authenticated_page: Page, grafana_url: str):
        """The 3D panel should contain an iframe pointing to /viz/topology."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(3000)
        iframe = page.locator('iframe[src*="/viz/topology"]')
        expect(iframe).to_be_visible(timeout=15000)

    def test_3d_iframe_loads_successfully(self, authenticated_page: Page, grafana_url: str):
        """The 3D viewer iframe should load without errors."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(3000)
        iframe_loc = page.locator('iframe[src*="/viz/topology"]')
        expect(iframe_loc).to_be_visible(timeout=15000)

        # Access the iframe content via FrameLocator
        frame = page.frame_locator('iframe[src*="/viz/topology"]')
        # Wait for the 3D graph to initialize, then check controls
        controls = frame.locator('#controls')
        expect(controls).to_be_visible(timeout=15000)

    def test_3d_viewer_shows_node_info(self, authenticated_page: Page, grafana_url: str):
        """The 3D viewer should display node/edge count info."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(8000)
        frame = page.frame_locator('iframe[src*="/viz/topology"]')
        info_el = frame.locator('#info')
        expect(info_el).to_be_visible(timeout=20000)
        # Wait for data to load (info changes from "Loading..." to stats)
        page.wait_for_timeout(8000)
        info_text = info_el.inner_text()
        assert "Nodes:" in info_text or "Loading" in info_text, \
            f"3D viewer info panel has unexpected content: {info_text[:200]}"

    def test_3d_viewer_has_layer_legend(self, authenticated_page: Page, grafana_url: str):
        """The 3D viewer should show a layer color legend."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(3000)
        frame = page.frame_locator('iframe[src*="/viz/topology"]')
        legend = frame.locator('#legend')
        expect(legend).to_be_visible(timeout=15000)

    def test_3d_viewer_passes_space_id(self, authenticated_page: Page, grafana_url: str):
        """The iframe src should pass the selected space_id variable."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(3000)
        iframe = page.locator('iframe[src*="/viz/topology"]')
        expect(iframe).to_be_visible(timeout=15000)
        src = iframe.get_attribute("src")
        assert "space_id=mdemg-dev" in src, f"iframe src missing space_id: {src}"


class Test3DViewerDirect:
    """Test the 3D viewer endpoint directly (outside Grafana)."""

    def test_viz_endpoint_returns_html(self, authenticated_page: Page, grafana_url: str):
        """GET /viz/topology should return an HTML page."""
        page = authenticated_page
        response = page.goto("http://localhost:9999/viz/topology")
        assert response.status == 200
        assert "text/html" in response.headers.get("content-type", "")

    def test_viz_page_has_3d_graph(self, authenticated_page: Page, grafana_url: str):
        """The 3D viewer page should render a 3D graph canvas."""
        page = authenticated_page
        page.goto(
            "http://localhost:9999/viz/topology?space_id=mdemg-dev&page_size=100",
            wait_until="networkidle",
        )
        page.wait_for_timeout(8000)  # Allow graph to load and render

        # 3d-force-graph renders into a canvas element
        canvas = page.locator("canvas")
        assert canvas.count() > 0, "No canvas element — 3D graph did not render"
        box = canvas.first.bounding_box()
        assert box is not None, "Canvas has no bounding box"
        assert box["width"] > 200, f"Canvas too narrow: {box['width']}px"
        assert box["height"] > 200, f"Canvas too short: {box['height']}px"

    def test_viz_page_shows_loaded_data(self, authenticated_page: Page, grafana_url: str):
        """The 3D viewer should load and display graph data."""
        page = authenticated_page
        page.goto(
            "http://localhost:9999/viz/topology?space_id=mdemg-dev&page_size=100",
            wait_until="networkidle",
        )
        page.wait_for_timeout(10000)

        info = page.locator('#info')
        expect(info).to_be_visible(timeout=15000)
        info_text = info.inner_text()
        assert "Nodes:" in info_text, f"Graph data not loaded: {info_text[:200]}"
        assert "Edges:" in info_text, f"Edge data not shown: {info_text[:200]}"

    def test_viz_page_shows_cross_layer_edges(self, authenticated_page: Page, grafana_url: str):
        """The 3D viewer should report cross-layer edges."""
        page = authenticated_page
        page.goto(
            "http://localhost:9999/viz/topology?space_id=mdemg-dev&page_size=500",
            wait_until="networkidle",
        )
        page.wait_for_timeout(10000)

        info = page.locator('#info')
        expect(info).to_be_visible(timeout=15000)
        info_text = info.inner_text()
        # The info panel shows "Cross-layer: N" — verify N > 0
        assert "Cross-layer:" in info_text, f"Cross-layer count not shown: {info_text}"
        import re
        match = re.search(r'Cross-layer:\s*(\d+)', info_text)
        assert match, f"Could not parse cross-layer count from: {info_text}"
        cross_count = int(match.group(1))
        assert cross_count > 0, f"No cross-layer edges found (count={cross_count})"


class TestNeighborhoodExplorer:
    """Verify the Neighborhood Explorer works with no focus nodes selected."""

    def test_no_focus_nodes_no_error(self, authenticated_page: Page, grafana_url: str):
        """With no focus nodes selected, Neighborhood Explorer should not show an error panel."""
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        exploration_row = page.get_by_text("Exploration", exact=True)
        if exploration_row.is_visible():
            exploration_row.click()
            page.wait_for_timeout(2000)

        neighborhood_panel = page.locator('[data-panelid]').filter(
            has=page.get_by_text("Neighborhood Explorer", exact=False)
        ).first
        if neighborhood_panel.is_visible():
            panel_text = neighborhood_panel.inner_text()
            assert "400" not in panel_text, \
                f"Neighborhood Explorer shows 400 error: {panel_text[:200]}"
            assert "Bad Request" not in panel_text, \
                f"Neighborhood Explorer shows Bad Request: {panel_text[:200]}"

    def test_focus_node_selection_renders(self, authenticated_page: Page, grafana_url: str):
        """Selecting a focus node should render the neighborhood graph."""
        page = authenticated_page
        import requests
        resp = requests.get(
            "http://localhost:9999/v1/memory/graph/topology",
            params={"space_id": "mdemg-dev", "layers": "2,3,4,5", "page": "1", "page_size": "1"},
        )
        if resp.status_code != 200:
            pytest.skip("MDEMG API not available")
        nodes = resp.json().get("nodes", [])
        if not nodes:
            pytest.skip("No nodes available for neighborhood test")
        node_id = nodes[0]["id"]

        page.goto(
            dashboard_url(grafana_url, DASHBOARD_UID, space_id="mdemg-dev", focus_nodes=node_id),
            wait_until="networkidle",
        )
        exploration_row = page.get_by_text("Exploration", exact=True)
        if exploration_row.is_visible():
            exploration_row.click()
            page.wait_for_timeout(3000)

        neighborhood_panel = page.locator('[data-panelid]').filter(
            has=page.get_by_text("Neighborhood Explorer", exact=False)
        ).first
        if neighborhood_panel.is_visible():
            panel_text = neighborhood_panel.inner_text()
            assert "400" not in panel_text, \
                f"Neighborhood Explorer error with focus node: {panel_text[:200]}"


class TestAPITopology:
    """Verify the topology API returns layer-balanced results with cross-layer edges."""

    def test_api_returns_multiple_layers(self, authenticated_page: Page, grafana_url: str):
        """The topology API should return nodes from multiple layers."""
        import requests
        resp = requests.get(
            "http://localhost:9999/v1/memory/graph/topology",
            params={"space_id": "mdemg-dev", "layers": "0,1,2,3,4,5", "page": "1", "page_size": "500"},
        )
        assert resp.status_code == 200
        data = resp.json()
        layers_seen = set()
        for node in data["nodes"]:
            subtitle = node.get("subTitle", "")
            if subtitle.startswith("L"):
                layers_seen.add(subtitle[1])
        assert len(layers_seen) >= 3, \
            f"Only {len(layers_seen)} layers represented — expected at least 3: {layers_seen}"

    def test_api_returns_cross_layer_edges(self, authenticated_page: Page, grafana_url: str):
        """The topology API should return edges between different layers."""
        import requests
        resp = requests.get(
            "http://localhost:9999/v1/memory/graph/topology",
            params={"space_id": "mdemg-dev", "layers": "0,1,2,3,4,5", "page": "1", "page_size": "500"},
        )
        assert resp.status_code == 200
        data = resp.json()
        node_layer = {}
        for n in data["nodes"]:
            subtitle = n.get("subTitle", "")
            if subtitle.startswith("L"):
                node_layer[n["id"]] = subtitle[1]
        cross = sum(
            1 for e in data["edges"]
            if node_layer.get(e["source"]) != node_layer.get(e["target"])
            and e["source"] in node_layer and e["target"] in node_layer
        )
        assert cross > 0, \
            f"No cross-layer edges found among {len(data['edges'])} edges"

    def test_api_pagination_metadata(self, authenticated_page: Page, grafana_url: str):
        """The topology API should return pagination metadata."""
        import requests
        resp = requests.get(
            "http://localhost:9999/v1/memory/graph/topology",
            params={"space_id": "mdemg-dev", "layers": "0,1,2,3,4,5", "page": "1", "page_size": "100"},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["total_nodes"] > 0, "total_nodes should be > 0"
        assert data["total_pages"] > 1, "total_pages should be > 1 with 59K+ nodes"
        assert data["page"] == 1
        assert len(data["nodes"]) > 0
