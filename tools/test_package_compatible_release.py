import copy
import unittest

import package_compatible_release as package


def info(site='kittyapi', version='2.9.1-kitty.1'):
    return {'Path': 'github.com/Wei-Shaw/sub2api/cmd/server',
            'Main': {'Path': 'github.com/Wei-Shaw/sub2api'},
            'Settings': [{'Key': 'GOOS', 'Value': 'linux'}, {'Key': 'GOARCH', 'Value': 'amd64'},
                         {'Key': '-ldflags', 'Value': f'-s -w -X main.Version={version} -X {package.SITE_SYMBOL}={site}'}]}


class BuildIdentityTests(unittest.TestCase):
    def validate(self, value, site='kittyapi', version='2.9.1-kitty.1'):
        return package.validate_build_info(value, site, version, 'linux', 'amd64')

    def test_each_site_has_its_own_repository(self):
        for site in package.REPOSITORIES:
            with self.subTest(site=site):
                self.assertEqual(self.validate(info(site), site)['release_repository'], package.REPOSITORIES[site])

    def test_cross_site_binary_rejected(self):
        for site, other in [('kittyapi', 'kittyyyds'), ('kittyyyds', 'kittyapi')]:
            with self.subTest(site=site), self.assertRaisesRegex(ValueError, 'site injection'):
                self.validate(info(other), site)

    def test_missing_injection_does_not_infer_from_default_or_incidental_word(self):
        for site in package.REPOSITORIES:
            value = info(site)
            value['Settings'][2]['Value'] = '-X main.Version=2.9.1-kitty.1 -X main.Description=' + site
            with self.subTest(site=site), self.assertRaisesRegex(ValueError, 'site injection'):
                self.validate(value, site)

    def test_unknown_site_rejected(self):
        with self.assertRaisesRegex(ValueError, 'unknown release site'):
            self.validate(info('other'), 'other')

    def test_version_mismatch_rejected(self):
        with self.assertRaisesRegex(ValueError, 'version injection'):
            self.validate(info(version='2.8.18-kitty.4'))

    def test_wrong_platform_rejected(self):
        for index in (0, 1):
            value = info()
            value['Settings'][index]['Value'] = 'other'
            with self.subTest(index=index), self.assertRaisesRegex(ValueError, 'OS and architecture'):
                self.validate(value)

    def test_duplicate_bindings_rejected_even_if_same_value(self):
        value = info()
        value['Settings'][2]['Value'] += f' -X {package.SITE_SYMBOL}=kittyapi'
        with self.assertRaisesRegex(ValueError, 'duplicate build-time'):
            self.validate(value)

    def test_quoted_and_equals_ldflags_are_understood(self):
        value = info()
        value['Settings'][2]['Value'] = f'-s -X=main.Version=2.9.1-kitty.1 -X "{package.SITE_SYMBOL}=kittyapi"'
        self.assertEqual(self.validate(value)['site_id'], 'kittyapi')

    def test_duplicate_or_malformed_settings_rejected(self):
        value = info()
        value['Settings'].append(copy.deepcopy(value['Settings'][0]))
        with self.assertRaisesRegex(ValueError, 'ambiguous'):
            self.validate(value)
        value['Settings'] = [{'Key': 'GOOS', 'Value': None}]
        with self.assertRaisesRegex(ValueError, 'invalid'):
            self.validate(value)

    def test_incomplete_ldflag_rejected(self):
        value = info()
        value['Settings'][2]['Value'] += ' -X'
        with self.assertRaisesRegex(ValueError, 'incomplete'):
            self.validate(value)

    def test_unrelated_binary_rejected(self):
        for key in ('Path', 'Main'):
            value = info()
            value[key] = 'other' if key == 'Path' else {'Path': 'other'}
            with self.subTest(key=key), self.assertRaisesRegex(ValueError, 'not the Sub2API'):
                self.validate(value)

    def test_trimpath_cannot_silently_skip_identity_proof(self):
        value = info()
        value['Settings'] = value['Settings'][:2] + [{'Key': '-trimpath', 'Value': 'true'}]
        with self.assertRaisesRegex(ValueError, 'without -trimpath'):
            self.validate(value)

    def test_malformed_metadata_structure_is_rejected(self):
        for value in (None, [], {'Main': [], 'Settings': []}, {'Main': {}, 'Settings': {}}):
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, 'invalid structure'):
                self.validate(value)


if __name__ == '__main__':
    unittest.main(verbosity=2)
