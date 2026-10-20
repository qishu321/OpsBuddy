// Generates the Windows manifest, version metadata and application icon.
package main

import (
	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
	"image"
	"image/color"
	"log"
	"os"
)

func main() {
	resources := winres.ResourceSet{}
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	rounded := func(x0, y0, x1, y1, r int, c color.RGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				cx, cy := x, y
				if x < x0+r {
					cx = x0 + r
				} else if x >= x1-r {
					cx = x1 - r - 1
				}
				if y < y0+r {
					cy = y0 + r
				} else if y >= y1-r {
					cy = y1 - r - 1
				}
				dx, dy := x-cx, y-cy
				if dx*dx+dy*dy <= r*r {
					img.SetRGBA(x, y, c)
				}
			}
		}
	}
	rounded(27, 28, 122, 123, 24, color.RGBA{123, 154, 240, 255})
	rounded(132, 28, 227, 123, 24, color.RGBA{82, 120, 221, 255})
	rounded(132, 133, 227, 228, 24, color.RGBA{49, 87, 188, 255})
	icon, err := winres.NewIconFromResizedImage(img, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err = resources.SetIcon(winres.RT_ICON, icon); err != nil {
		log.Fatal(err)
	}
	manifest, err := winres.AppManifestFromXML([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly manifestVersion="1.0" xmlns="urn:schemas-microsoft-com:asm.v1" xmlns:asmv3="urn:schemas-microsoft-com:asm.v3">
 <assemblyIdentity type="win32" name="OpsBuddy.Desktop" version="0.17.1.0" processorArchitecture="*"/>
 <dependency><dependentAssembly><assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency>
 <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges><requestedExecutionLevel level="asInvoker" uiAccess="false"/></requestedPrivileges></security></trustInfo>
 <asmv3:application><asmv3:windowsSettings><dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware><dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2,PerMonitor</dpiAwareness><longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware></asmv3:windowsSettings></asmv3:application>
</assembly>`))
	if err != nil {
		log.Fatal(err)
	}
	resources.SetManifest(manifest)
	info := version.Info{FileVersion: [4]uint16{0, 17, 1, 0}, ProductVersion: [4]uint16{0, 17, 1, 0}, Type: version.App}
	for key, value := range map[string]string{version.ProductName: "OpsBuddy", version.FileDescription: "OpsBuddy Desktop", version.OriginalFilename: "OpsBuddy.exe", version.FileVersion: "0.17.1", version.ProductVersion: "0.17.1"} {
		if err = info.Set(version.LangDefault, key, value); err != nil {
			log.Fatal(err)
		}
	}
	resources.SetVersionInfo(info)
	f, err := os.Create("cmd/opsbuddy-desktop/rsrc_windows_amd64.syso")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err = resources.WriteObject(f, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
}
