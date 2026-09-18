# Changelog

## [0.8.0](https://github.com/eriksaulnier/loupe/compare/v0.7.0...v0.8.0) (2026-09-18)


### Features

* **publish:** let the human write the review's opening ([#19](https://github.com/eriksaulnier/loupe/issues/19)) ([cde9bb6](https://github.com/eriksaulnier/loupe/commit/cde9bb607a69d9c02bbbf8ee6d54128e3dc01ad8))
* show severity and order every surface by it ([#18](https://github.com/eriksaulnier/loupe/issues/18)) ([8561907](https://github.com/eriksaulnier/loupe/commit/85619078485ded5416994bd83450b876a45280dd))
* **tui:** reinstate a withdrawn finding from review ([#13](https://github.com/eriksaulnier/loupe/issues/13)) ([fcd0b12](https://github.com/eriksaulnier/loupe/commit/fcd0b1216911abf00ae0932b493323dbb99f3bf5))

## [0.7.0](https://github.com/eriksaulnier/loupe/compare/v0.6.0...v0.7.0) (2026-09-17)


### Features

* **cli:** add loupe show --diff ([#6](https://github.com/eriksaulnier/loupe/issues/6)) ([c8c2635](https://github.com/eriksaulnier/loupe/commit/c8c2635379a4c9edd921c5a146372e66ee90dc42))

## [0.6.0](https://github.com/eriksaulnier/loupe/compare/v0.5.0...v0.6.0) (2026-09-16)


### Features

* a footer for the people reading the review ([d80d6e7](https://github.com/eriksaulnier/loupe/commit/d80d6e763b42dbf6b9a6795e0fc4acf24a9d19da))
* add impact, verified, references and the reviewer's model ([9a048ff](https://github.com/eriksaulnier/loupe/commit/9a048ff03a0ae068b72d5b9ce4807d57e917d9bb))
* publish unattended for any review pipeline ([32b676a](https://github.com/eriksaulnier/loupe/commit/32b676aef39c26c8ca187cbaffd16081f27ac5b3))
* **review:** ask for a round with a label ([e7cb8d2](https://github.com/eriksaulnier/loupe/commit/e7cb8d2f92607b1d686cbbe2f4d39376f1e029f7))

## [0.5.0](https://github.com/eriksaulnier/loupe/compare/v0.4.0...v0.5.0) (2026-09-15)


### Features

* add loupe handoff and ship the plugin to codex and pi ([36bbb02](https://github.com/eriksaulnier/loupe/commit/36bbb020124b30352213761e6cbb744bf7a46825))
* **render:** drop the blocking callout and body finding dots ([9533218](https://github.com/eriksaulnier/loupe/commit/953321802154501e485e89a7cbeec46a9a65beed))
* **tui:** edit a finding's label and blocking in review ([03964b6](https://github.com/eriksaulnier/loupe/commit/03964b68efbfa31ebe95978a274666c303e364ad))


### Bug Fixes

* **tui:** drop keys typed as review opens ([3c157b5](https://github.com/eriksaulnier/loupe/commit/3c157b581359ae10432ffd1dfca02b14f4f28d48))
* **tui:** wrap the footer to two lines before dropping hints ([dc3f7b0](https://github.com/eriksaulnier/loupe/commit/dc3f7b05b3172bb67cadd1d3a44e8e26bf930506))

## [0.4.0](https://github.com/eriksaulnier/loupe/compare/v0.3.0...v0.4.0) (2026-09-15)


### Features

* **plugin:** open loupe review in a herdr split ([b95b13a](https://github.com/eriksaulnier/loupe/commit/b95b13a854277706d0dcc553b8cf926043b58176))
* **render:** lead each section heading with its chip's dot ([d15c1cd](https://github.com/eriksaulnier/loupe/commit/d15c1cde8855950caccf536a3aa7592c23c3a966))
* **tui:** close each header block with a dim rule ([2bf78af](https://github.com/eriksaulnier/loupe/commit/2bf78afc58729857901c30e09e4c5fc1f0810bb0))
* **tui:** flatten the review interface and lead with arrow keys ([49288cf](https://github.com/eriksaulnier/loupe/commit/49288cf15d9ace8be215392a23e64b7e87fb50d6))


### Bug Fixes

* **tui:** wrap finding bodies with glamour v2 and mark open notes ([050971e](https://github.com/eriksaulnier/loupe/commit/050971e7e37546dc57b01dc58d71553b142de8a8))

## [0.3.0](https://github.com/eriksaulnier/loupe/compare/v0.2.0...v0.3.0) (2026-09-14)


### Features

* **capture:** record an optional source in the footer and loupe-meta ([f0b4e84](https://github.com/eriksaulnier/loupe/commit/f0b4e84eb37aed6414d3f675892b4befb2a1be57))
* **publish:** publish at the captured head when the head moved forward ([cad8f41](https://github.com/eriksaulnier/loupe/commit/cad8f419881a1072291c518988116feba4f83936))
* **render:** recolor issue and suggestion dots ([97c640f](https://github.com/eriksaulnier/loupe/commit/97c640fcd6de616095666fde7bf678273617e6a4))
* **render:** show the callout only when findings block ([cbfe2a3](https://github.com/eriksaulnier/loupe/commit/cbfe2a3013ee7dcccf279bf3a5b4742eff817ef2))


### Bug Fixes

* **publish:** correct the moved-head publish against live GitHub ([6b2a129](https://github.com/eriksaulnier/loupe/commit/6b2a129cb9eee176c22a8f9d5fe620fc7f835e0d))
* **publish:** drop the capture round from the published report ([626fe4b](https://github.com/eriksaulnier/loupe/commit/626fe4b24c104fc152b7ce67ed34425f31fd4fb2))
* **publish:** number the published round by publications ([3195a84](https://github.com/eriksaulnier/loupe/commit/3195a84af3a1c827cd5ca0c2d75e2d0692366d4f))

## [0.2.0](https://github.com/eriksaulnier/loupe/compare/v0.1.0...v0.2.0) (2026-09-14)


### Features

* **wait:** block until the human hands notes back or publishes ([9f01cd3](https://github.com/eriksaulnier/loupe/commit/9f01cd3c09ed4e297ce6059e3b9e2de3e8f9754d))

## 0.1.0 (2026-09-14)


### Features

* **cli:** icons on one-shot output and the LOUPE_ICONS override ([a04db16](https://github.com/eriksaulnier/loupe/commit/a04db1654279f6438ab2d43385c6de8ee55197df))
* **style:** nerd icon tier, catppuccin palette and powerline band ([92c31cd](https://github.com/eriksaulnier/loupe/commit/92c31cd2f28eea1b57e2703c666bc0e654c9871f))


### Bug Fixes

* **tui:** keep wrap indents out of the collapsed summary ([893a886](https://github.com/eriksaulnier/loupe/commit/893a8864b24913ebf76e49155ddb70b752284bf2))
* **tui:** size the file-diff band by the brand's real width ([8b85566](https://github.com/eriksaulnier/loupe/commit/8b85566f3f77a1caba94a0dba4cf70be4c4f0679))
