package export

import (
	"html/template"
	"io/fs"
	"os"
	"path/filepath"

	cp "github.com/otiai10/copy"
	"github.com/waynezhang/foto/internal/cache"
	"github.com/waynezhang/foto/internal/config"
	"github.com/waynezhang/foto/internal/files"
	"github.com/waynezhang/foto/internal/images"
	"github.com/waynezhang/foto/internal/indexer"
	"github.com/waynezhang/foto/internal/log"
	mm "github.com/waynezhang/foto/internal/minimize"
	"github.com/waynezhang/foto/internal/utils"
)

type DefaultExportContext struct{}

func (ctx DefaultExportContext) cleanDirectory(outputPath string) error {
	return files.PruneDirectory(outputPath)
}

func (ctx DefaultExportContext) buildIndex(cfg config.Config) ([]indexer.Section, error) {
	return indexer.Build(cfg.GetSectionMetadata(), cfg.GetExtractOption())
}

func (ctx DefaultExportContext) exportPhotos(sections []indexer.Section, outputPath string, cache cache.Cache, progressFunc ProgressFunc) {
	if err := files.EnsureDirectory(outputPath); err != nil {
		utils.CheckFatalError(err, "Failed to prepare output directory")
		return
	}

	for _, s := range sections {
		for _, set := range s.ImageSets {
			srcPath := filepath.Join(s.Folder, set.FileName)

			log.Debug("Processing image %s", srcPath)
			if progressFunc != nil {
				progressFunc(srcPath)
			}

			thumbnailPath := files.OutputPhotoThumbnailFilePath(outputPath, s.Slug, srcPath)
			err := resizeImageAndCache(srcPath, thumbnailPath, set.ThumbnailSize.Width, cache)
			utils.CheckFatalError(err, "Failed to generate thumbnail image")

			originalPath := files.OutputPhotoOriginalFilePath(outputPath, s.Slug, srcPath)
			err = resizeImageAndCache(srcPath, originalPath, set.OriginalSize.Width, cache)
			utils.CheckFatalError(err, "Failed to generate original image")
		}
	}
}

func (ctx DefaultExportContext) generateIndexHtml(cfg config.Config, templatePath string, sections []indexer.Section, path string, minimizer mm.Minimizer) {
	f, err := os.Create(path)
	utils.CheckFatalError(err, "Failed to create index file.")
	defer f.Close()

	tmpl := template.Must(template.ParseFiles(templatePath))
	err = tmpl.Execute(f, struct {
		Config   map[string]interface{}
		Sections []indexer.Section
	}{
		cfg.AllSettings(),
		sections,
	})
	utils.CheckFatalError(err, "Failed to generate index page.")

	_ = minimizer.MinimizeFile(path, path)
}

func (ctx DefaultExportContext) processOtherFolders(folders []string, outputPath string, minimizer mm.Minimizer, messageFunc func(src string, dst string)) {
	for _, folder := range folders {
		targetFolder := filepath.Join(outputPath, folder)
		if messageFunc != nil {
			messageFunc(folder, targetFolder)
		}

		if err := cp.Copy(folder, targetFolder); err != nil {
			log.Fatal("Failed to copy folder %s to %s (%s).", folder, targetFolder, err)
		}
		_ = filepath.WalkDir(targetFolder, func(path string, d fs.DirEntry, err error) error {
			if minimizer.Minimizable(path) {
				return minimizer.MinimizeFile(path, path)
			}
			return nil
		})
	}
}

func resizeImageAndCache(src string, to string, width int, cache cache.Cache) error {
	cached := cache.CachedImage(src, width)
	if cached != nil {
		log.Debug("Found cached image for %s", src)
		err := cp.Copy(*cached, to)
		if err == nil {
			return nil
		}
	}

	err := images.ResizeImage(src, to, width)
	if err != nil {
		return err
	}

	cache.AddImage(src, width, to)

	return nil
}