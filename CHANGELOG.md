# Changelog

## [0.3.0](https://github.com/eriksaulnier/loupe/compare/v0.2.0...v0.3.0) (2026-09-14)


### Features

* **capture:** record an optional source in the footer and loupe-meta ([e503b30](https://github.com/eriksaulnier/loupe/commit/e503b30cee1caf78104582de4f6b4ca2e0ea21ec))
* **publish:** publish at the captured head when the head moved forward ([c203444](https://github.com/eriksaulnier/loupe/commit/c20344496b622dcd5cce3e9c5f75b4e1d2881cbc))
* **render:** recolor issue and suggestion dots ([3f5750a](https://github.com/eriksaulnier/loupe/commit/3f5750a22672aac39e805dff54278b91322d6acf))
* **render:** show the callout only when findings block ([6c22d8c](https://github.com/eriksaulnier/loupe/commit/6c22d8cafa0a9dd3641cd83e9537721d4dea5b37))


### Bug Fixes

* **publish:** correct the moved-head publish against live GitHub ([2d41922](https://github.com/eriksaulnier/loupe/commit/2d41922d3ee2c44424d71bcb01a017cd3a56067b))
* **publish:** drop the capture round from the published report ([029df1b](https://github.com/eriksaulnier/loupe/commit/029df1ba1610a25f6da18ba77aea03252ec0f374))
* **publish:** number the published round by publications ([72fcda3](https://github.com/eriksaulnier/loupe/commit/72fcda375c627f98c1ff054a765c6c466b01bd0c))

## [0.2.0](https://github.com/eriksaulnier/loupe/compare/v0.1.0...v0.2.0) (2026-09-14)


### Features

* **wait:** block until the human hands notes back or publishes ([666aa25](https://github.com/eriksaulnier/loupe/commit/666aa25223d3efe09c0d4db8b1108be58dd87bb4))

## 0.1.0 (2026-09-14)


### Features

* **cli:** icons on one-shot output and the LOUPE_ICONS override ([5dc647c](https://github.com/eriksaulnier/loupe/commit/5dc647c22ca2c14b00d73e3a0c5bcfa14992b7c2))
* **style:** nerd icon tier, catppuccin palette and powerline band ([ff8786c](https://github.com/eriksaulnier/loupe/commit/ff8786c5b8c6c8b2c57717f44868eac8e21e59bb))


### Bug Fixes

* **tui:** keep wrap indents out of the collapsed summary ([402c6c1](https://github.com/eriksaulnier/loupe/commit/402c6c170407b51f3ab960aa2b9ddda47d96c4fb))
* **tui:** size the file-diff band by the brand's real width ([ddf7cd4](https://github.com/eriksaulnier/loupe/commit/ddf7cd4bd005109753db79583b2bf6a6a88154f4))
