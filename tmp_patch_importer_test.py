import pathlib


def test_patch_importer():
    target = pathlib.Path("internal/itunes/service/importer.go")
    text = target.read_text()
    old = "\t\tartist := strings.TrimSpace(track.Artist)\n\t\talbum := strings.TrimSpace(track.Album)\n\t\tif album == \"\" {\n\t\t\talbum = strings.TrimSpace(track.Name)\n\t\t}\n\t\tkey := artist + \"|\" + album"
    new = "\t\tartist := strings.TrimSpace(track.Artist)\n\t\talbum := strings.TrimSpace(track.Album)\n\t\tif album == \"\" {\n\t\t\ttrackName := strings.TrimSpace(track.Name)\n\t\t\tstripped := stripChapterPrefix(trackName)\n\t\t\tif stripped != \"\" {\n\t\t\t\talbum = stripped\n\t\t\t} else {\n\t\t\t\talbum = trackName\n\t\t\t}\n\t\t}\n\t\tkey := artist + \"|\" + album"

    if old not in text:
        raise AssertionError("target pattern not found")

    target.write_text(text.replace(old, new, 1))
    pathlib.Path(__file__).unlink()
