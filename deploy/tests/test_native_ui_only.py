from __future__ import annotations

import json
import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


class NativeUiOnlyTest(unittest.TestCase):
    def test_retired_injection_assets_are_absent(self) -> None:
        retired_paths = (
            "deploy/kitty-site/kitty-auth.css",
            "deploy/kitty-site/kitty-auth.js",
            "deploy/kitty-site/kitty-head.js",
            "deploy/kitty-site/kitty-home-i18n.js",
            "deploy/kitty-site/deploy-global-brand.sh",
            "frontend/build-kitty-native-candidate.mjs",
            "frontend/serve-kitty-native-demo.mjs",
        )

        for relative_path in retired_paths:
            self.assertFalse((ROOT / relative_path).exists(), relative_path)

    def test_frontend_has_no_retired_build_or_public_routes(self) -> None:
        package = json.loads((ROOT / "frontend/package.json").read_text(encoding="utf-8"))
        scripts = package.get("scripts", {})
        self.assertNotIn("build:kitty-native-candidate", scripts)
        self.assertNotIn("preview:kitty-native-demo", scripts)

        router = (ROOT / "frontend/src/router/index.ts").read_text(encoding="utf-8")
        self.assertNotIn("/legacy-home", router)
        self.assertNotIn("/heyroute-preview", router)
        self.assertIn("@/views/HomeLandingView.vue", router)

    def test_proxy_documentation_forbids_ui_injection(self) -> None:
        readme = (ROOT / "deploy/kitty-site/README.md").read_text(encoding="utf-8")
        self.assertIn("must\nserve the application response unchanged", readme)
        self.assertIn("must return `404`", readme)

    def test_brand_font_is_self_hosted(self) -> None:
        package = json.loads((ROOT / "frontend/package.json").read_text(encoding="utf-8"))
        self.assertEqual(
            package.get("dependencies", {}).get("@fontsource-variable/montserrat"),
            "5.3.0",
        )

        main = (ROOT / "frontend/src/main.ts").read_text(encoding="utf-8")
        self.assertIn("import '@fontsource-variable/montserrat'", main)

        for relative_path in (
            "frontend/src/components/auth/HeyRouteAuthLayout.vue",
            "frontend/src/components/home/heyroute/HeyRouteLandingPage.vue",
        ):
            source = (ROOT / relative_path).read_text(encoding="utf-8")
            self.assertNotIn("fonts.googleapis.com", source, relative_path)
            self.assertIn("'Montserrat Variable'", source, relative_path)

    def test_scoped_styles_do_not_truncate_global_descendants(self) -> None:
        components = ROOT / "frontend/src/components/home/heyroute"
        invalid_selector = re.compile(r":global\([^\r\n]+\)\s+[.#[]")

        for component in components.glob("*.vue"):
            source = component.read_text(encoding="utf-8")
            self.assertIsNone(
                invalid_selector.search(source),
                f"{component.name} mixes a global parent with a scoped descendant",
            )


if __name__ == "__main__":
    unittest.main()
