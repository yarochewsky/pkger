package mem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/markbates/pkger/here"
	"github.com/markbates/pkger/internal/maps"
	"github.com/markbates/pkger/pkging"
)

var _ pkging.Pkger = &Pkger{}

func WithInfo(fx *Pkger, infos ...here.Info) {
	for _, info := range infos {
		fx.infos.Store(info.ImportPath, info)
	}
}

func New(info here.Info) (*Pkger, error) {
	f := &Pkger{
		infos: &maps.Infos{},
		paths: &maps.Paths{
			Current: info,
		},
		files:   &maps.Files{},
		current: info,
	}
	f.infos.Store(info.ImportPath, info)
	f.MkdirAll("/", 0755)
	return f, nil
}

type Pkger struct {
	infos   *maps.Infos
	paths   *maps.Paths
	files   *maps.Files
	current here.Info
}

func (f *Pkger) Abs(p string) (string, error) {
	pt, err := f.Parse(p)
	if err != nil {
		return "", err
	}
	return f.AbsPath(pt)
}

func (f *Pkger) AbsPath(pt pkging.Path) (string, error) {
	return pt.String(), nil
}

func (f *Pkger) Current() (here.Info, error) {
	return f.current, nil
}

func (f *Pkger) Info(p string) (here.Info, error) {
	info, ok := f.infos.Load(p)
	if !ok {
		return info, fmt.Errorf("no such package %q", p)
	}

	return info, nil
}

func (f *Pkger) Parse(p string) (pkging.Path, error) {
	return f.paths.Parse(p)
}

func (fx *Pkger) Remove(name string) error {
	pt, err := fx.Parse(name)
	if err != nil {
		return err
	}

	if _, ok := fx.files.Load(pt); !ok {
		return &os.PathError{"remove", pt.String(), fmt.Errorf("no such file or directory")}
	}

	fx.files.Delete(pt)
	return nil
}

func (fx *Pkger) RemoveAll(name string) error {
	pt, err := fx.Parse(name)
	if err != nil {
		return err
	}

	fx.files.Range(func(key pkging.Path, file pkging.File) bool {
		if strings.HasPrefix(key.Name, pt.Name) {
			fx.files.Delete(key)
		}
		return true
	})

	fx.paths.Range(func(key string, value pkging.Path) bool {
		if strings.HasPrefix(key, pt.Name) {
			fx.paths.Delete(key)
		}
		return true
	})
	return nil
}

func (fx *Pkger) Create(name string) (pkging.File, error) {
	fx.MkdirAll("/", 0755)
	pt, err := fx.Parse(name)
	if err != nil {
		return nil, err
	}

	her, err := fx.Info(pt.Pkg)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(pt.Name)
	if _, err := fx.Stat(dir); err != nil {
		return nil, err
	}

	f := &File{
		path: pt,
		her:  her,
		info: &pkging.FileInfo{
			Details: pkging.Details{
				Name:    pt.Name,
				Mode:    0644,
				ModTime: pkging.ModTime(time.Now()),
			},
		},
		pkging: fx,
	}

	fx.files.Store(pt, f)

	return f, nil
}

func (fx *Pkger) MkdirAll(p string, perm os.FileMode) error {
	path, err := fx.Parse(p)
	if err != nil {
		return err
	}
	root := path.Name

	cur, err := fx.Current()
	if err != nil {
		return err
	}
	for root != "" {
		pt := pkging.Path{
			Pkg:  path.Pkg,
			Name: root,
		}
		if _, ok := fx.files.Load(pt); ok {
			root = filepath.Dir(root)
			if root == "/" || root == "\\" {
				break
			}
			continue
		}
		f := &File{
			pkging: fx,
			path:   pt,
			her:    cur,
			info: &pkging.FileInfo{
				Details: pkging.Details{
					Name:    pt.Name,
					Mode:    perm,
					ModTime: pkging.ModTime(time.Now()),
				},
			},
		}

		if err != nil {
			return err
		}
		f.info.Details.IsDir = true
		f.info.Details.Mode = perm
		if err := f.Close(); err != nil {
			return err
		}
		fx.files.Store(pt, f)
		root = filepath.Dir(root)
	}

	return nil

}

func (fx *Pkger) Open(name string) (pkging.File, error) {
	pt, err := fx.Parse(name)
	if err != nil {
		return nil, err
	}

	fl, ok := fx.files.Load(pt)
	if !ok {
		return nil, fmt.Errorf("could not open %s", name)
	}
	f, ok := fl.(*File)
	if !ok {
		return nil, fmt.Errorf("could not open %s", name)
	}
	nf := &File{
		pkging: fx,
		info:   pkging.WithName(f.info.Name(), f.info),
		path:   f.path,
		data:   f.data,
		her:    f.her,
	}

	return nf, nil
}

func (fx *Pkger) Stat(name string) (os.FileInfo, error) {
	pt, err := fx.Parse(name)
	if err != nil {
		return nil, err
	}
	f, ok := fx.files.Load(pt)
	if ok {
		return f.Stat()
	}
	return nil, fmt.Errorf("could not stat %s", pt)
}

func (f *Pkger) Walk(p string, wf filepath.WalkFunc) error {
	keys := f.files.Keys()

	pt, err := f.Parse(p)
	if err != nil {
		return err
	}

	skip := "!"

	for _, k := range keys {
		if !strings.HasPrefix(k.Name, pt.Name) {
			continue
		}
		if strings.HasPrefix(k.Name, skip) {
			continue
		}
		fl, ok := f.files.Load(k)
		if !ok {
			return fmt.Errorf("could not find %s", k)
		}
		fi, err := fl.Stat()
		if err != nil {
			return err
		}

		fi = pkging.WithName(strings.TrimPrefix(k.Name, pt.Name), fi)
		err = wf(k.String(), fi, nil)
		if err == filepath.SkipDir {

			skip = k.Name
			continue
		}

		if err != nil {
			return err
		}
	}
	return nil
}