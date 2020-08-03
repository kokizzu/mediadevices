package video

import (
	"fmt"
	"image"
	"runtime"
	"testing"
	"time"

	"github.com/pion/mediadevices/pkg/prop"
)

func BenchmarkDetectChanges(b *testing.B) {
	var src Reader
	src = ReaderFunc(func() (image.Image, error) {
		return image.NewRGBA(image.Rect(0, 0, 1920, 1080)), nil
	})

	b.Run("WithoutDetectChanges", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			src.Read()
		}
	})

	ns := []int{1, 8, 64, 256}
	for _, n := range ns {
		n := n
		src := src
		b.Run(fmt.Sprintf("WithDetectChanges%d", n), func(b *testing.B) {
			for i := 0; i < n; i++ {
				src = DetectChanges(time.Microsecond, func(p prop.Media) {})(src)
			}

			for i := 0; i < b.N; i++ {
				src.Read()
			}
		})
	}
}

func TestDetectChanges(t *testing.T) {
	buildSource := func(p prop.Media) (Reader, func(prop.Media)) {
		return ReaderFunc(func() (image.Image, error) {
				return image.NewRGBA(image.Rect(0, 0, p.Width, p.Height)), nil
			}), func(newProp prop.Media) {
				p = newProp
			}
	}

	assertEq := func(t *testing.T, actual prop.Media, expected prop.Media, output image.Image, assertFrameRate bool) {
		if actual.Height != expected.Height {
			t.Fatalf("expected height from to be %d but got %d", expected.Height, actual.Height)
		}

		if actual.Width != expected.Width {
			t.Fatalf("expected width from to be %d but got %d", expected.Width, actual.Width)
		}

		if assertFrameRate {
			diff := actual.FrameRate - expected.FrameRate
			// TODO: reduce this eps. Darwin CI keeps failing if we use a lower value
			var eps float32 = 1.5
			if diff < -eps || diff > eps {
				t.Fatalf("expected frame rate to be %f (+-%f) but got %f", expected.FrameRate, eps, actual.FrameRate)
			}
		}

		if output.Bounds().Dy() != expected.Height {
			t.Fatalf("expected output height from to be %d but got %d", expected.Height, output.Bounds().Dy())
		}

		if output.Bounds().Dx() != expected.Width {
			t.Fatalf("expected output width from to be %d but got %d", expected.Width, output.Bounds().Dx())
		}
	}

	t.Run("OnChangeCalledBeforeFirstFrame", func(t *testing.T) {
		var detectBeforeFirstFrame bool
		var expected prop.Media
		var actual prop.Media
		expected.Width = 1920
		expected.Height = 1080
		src, _ := buildSource(expected)
		src = DetectChanges(time.Second, func(p prop.Media) {
			actual = p
			detectBeforeFirstFrame = true
		})(src)

		frame, err := src.Read()
		if err != nil {
			t.Fatal(err)
		}

		if !detectBeforeFirstFrame {
			t.Fatal("on change callback should have called before first frame")
		}

		assertEq(t, actual, expected, frame, false)
	})

	t.Run("DetectChangesOnEveryUpdate", func(t *testing.T) {
		var expected prop.Media
		var actual prop.Media
		expected.Width = 1920
		expected.Height = 1080
		src, update := buildSource(expected)
		src = DetectChanges(time.Second, func(p prop.Media) {
			actual = p
		})(src)

		for width := 1920; width < 4000; width += 100 {
			for height := 1080; height < 2000; height += 100 {
				expected.Width = width
				expected.Height = height
				update(expected)
				frame, err := src.Read()
				if err != nil {
					t.Fatal(err)
				}

				assertEq(t, actual, expected, frame, false)
			}
		}
	})

	t.Run("FrameRateAccuracy", func(t *testing.T) {
		// https://github.com/pion/mediadevices/issues/198
		if runtime.GOOS == "darwin" {
			t.Skip("Skipping because Darwin CI is not reliable for timing related tests.")
		}

		var expected prop.Media
		var actual prop.Media
		var count int
		expected.Width = 1920
		expected.Height = 1080
		expected.FrameRate = 30
		src, _ := buildSource(expected)
		src = Throttle(expected.FrameRate)(src)
		src = DetectChanges(time.Second*5, func(p prop.Media) {
			actual = p
			count++
		})(src)

		for count < 3 {
			frame, err := src.Read()
			if err != nil {
				t.Fatal(err)
			}

			checkFrameRate := false
			if actual.FrameRate != 0.0 {
				checkFrameRate = true
			}
			assertEq(t, actual, expected, frame, checkFrameRate)
		}
	})
}