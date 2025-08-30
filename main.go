package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-vgo/robotgo"
	"github.com/nfnt/resize"
	"github.com/otiai10/gosseract"
	"github.com/vcaesar/imgo"
)

// --- CONFIGURACIÓN GENERAL ---
const DEBUG_MODE = true

// --- CONFIGURACIÓN TAREA: AYUDAR ALIADOS ---
const iconoAyudaFile = "resources/ayuda_icono.png"

var GUARDAR_PRIMERA_CAPTURA_AYUDA = true

const TOLERANCIA_COLOR uint32 = 20000
const TOLERANCIA_PIXEL_PORCENTAJE = 0.1

var AREA_BUSQUEDA_AYUDA = image.Rect(2290, 1500, 2310, 1520)

// --- CONFIGURACIÓN TAREA: REUNIÓN ---
const iconoReunionFile = "resources/reunion_icono.png"
const iconoMasVerdeFile = "resources/mas_verde_icono.png"
const iconoPartirFile = "resources/partir_icono.png"

var GUARDAR_PRIMERA_CAPTURA_REUNION = true

const TOLERANCIA_COLOR_REUNION uint32 = 30000   // Puedes ajustar esta tolerancia si es necesario
const TOLERANCIA_PIXEL_REUNION = 0.05           // Y también este porcentaje
const TOLERANCIA_COLOR_MAS_VERDE uint32 = 20000 // Puedes ajustar esta tolerancia si es necesario
const TOLERANCIA_PIXEL_MAS_VERDE = 0.05         // Y también este porcentaje
const TOLERANCIA_COLOR_PARTIR uint32 = 20000    // Puedes ajustar esta tolerancia si es necesario
const TOLERANCIA_PIXEL_PARTIR = 0.01            // Y también este porcentaje

var AREA_BUSQUEDA_REUNION = image.Rect(2408, 1090, 2413, 1237)
var AREA_BUSQUEDA_BOTON_PARTIR = image.Rect(1853, 1494, 1873, 1514)
var AREA_OCR_REUNION = image.Rect(2380, 1293, 2490, 1323)

// --- CONFIGURACIÓN DE BÚSQUEDA OPTIMIZADA DE BOTÓN VERDE ---
const (
	ALTO_TARJETA_REUNION     = 486
	ESPACIO_TARJETA_REUNION  = 30
	NUMERO_TARJETAS_VISIBLES = 3
)

var AREA_BUSQUEDA_BOTON_VERDE_INICIAL = image.Rect(2300, 420, 2320, 440)

// --- HELPERS ---
// absDiff calcula la diferencia absoluta entre dos valores uint32.
func absDiff(x, y uint32) uint32 {
	if x > y {
		return x - y
	}
	return y - x
}

func esTextoDeContador(texto string) bool {
	return strings.Contains(texto, ":")
}

// --- LÓGICA DE BÚSQUEDA MANUAL ---

// buscarIcono itera sobre un área específica de la pantalla para encontrar el icono,
// permitiendo una tolerancia de color y un porcentaje de píxeles no coincidentes.
func buscarIcono(pantalla image.Image, icono image.Image, areaBusqueda image.Rectangle, toleranciaColor uint32, toleranciaPixel float64) image.Point {
	bIcono := icono.Bounds()

	// Asegurarnos de que el icono no esté vacío
	if bIcono.Empty() {
		return image.Point{X: -1, Y: -1}
	}

	totalPixelesIcono := float64(bIcono.Dx() * bIcono.Dy())
	maxPixelesNoCoincidentes := int(totalPixelesIcono * toleranciaPixel)

	// Iteramos únicamente sobre el área de búsqueda definida.
	for y := areaBusqueda.Min.Y; y <= areaBusqueda.Max.Y; y++ {
		for x := areaBusqueda.Min.X; x <= areaBusqueda.Max.X; x++ {

			pixelesNoCoincidentes := 0

			// Comparamos cada píxel del icono con la región correspondiente de la pantalla.
			for iy := 0; iy < bIcono.Dy(); iy++ {
				for ix := 0; ix < bIcono.Dx(); ix++ {
					// Obtenemos los colores de ambos píxeles.
					r1, g1, b1, _ := pantalla.At(x+ix, y+iy).RGBA()
					r2, g2, b2, _ := icono.At(bIcono.Min.X+ix, bIcono.Min.Y+iy).RGBA()

					// Calculamos la diferencia total en los componentes RGB.
					diff := absDiff(r1, r2) + absDiff(g1, g2) + absDiff(b1, b2)

					// Si la diferencia es mayor que nuestra tolerancia, contamos un píxel no coincidente.
					if diff > toleranciaColor {
						pixelesNoCoincidentes++
					}
				}
			}

			// Si el número de píxeles no coincidentes está dentro de nuestra tolerancia, hemos encontrado el icono.
			if pixelesNoCoincidentes <= maxPixelesNoCoincidentes {
				return image.Point{X: x, Y: y}
			}
		}
	}

	return image.Point{X: -1, Y: -1}
}

// buscarBotonVerdeEnTarjetas implementa la búsqueda optimizada iterando por cada tarjeta.
func buscarBotonVerdeEnTarjetas(pantalla image.Image, icono image.Image) image.Point {
	desplazamientoTotal := 0
	for i := 0; i < NUMERO_TARJETAS_VISIBLES; i++ {
		// Calculamos el área de búsqueda para la tarjeta actual.
		areaActual := image.Rect(
			AREA_BUSQUEDA_BOTON_VERDE_INICIAL.Min.X,
			AREA_BUSQUEDA_BOTON_VERDE_INICIAL.Min.Y+desplazamientoTotal,
			AREA_BUSQUEDA_BOTON_VERDE_INICIAL.Max.X,
			AREA_BUSQUEDA_BOTON_VERDE_INICIAL.Max.Y+desplazamientoTotal,
		)

		pt := buscarIcono(pantalla, icono, areaActual, TOLERANCIA_COLOR_MAS_VERDE, TOLERANCIA_PIXEL_MAS_VERDE)
		if pt.X != -1 {
			return pt // Si lo encontramos, devolvemos el punto y terminamos.
		}

		// Calculamos el desplazamiento para la siguiente tarjeta.
		desplazamientoTotal += ALTO_TARJETA_REUNION + ESPACIO_TARJETA_REUNION
	}
	return image.Point{X: -1, Y: -1} // Si no lo encontramos en ninguna tarjeta.
}

// --- LÓGICA DE AUTOMATIZACIÓN ---

func buscarYAyudarAliados(wg *sync.WaitGroup, done <-chan bool, pausar chan bool) {
	defer wg.Done()
	fmt.Println("-> Hilo de ayuda a aliados INICIADO. Buscando el icono en el área optimizada...")

	iconoImg, err := imgo.Read(iconoAyudaFile)
	if err != nil {
		fmt.Printf("Error fatal: No se pudo cargar el fichero del icono de ayuda '%s'.\n", iconoAyudaFile)
		return
	}

	for {
		select {
		case <-done:
			fmt.Println("-> Hilo de ayuda a aliados DETENIDO.")
			return
		case enPausa := <-pausar:
			if enPausa {
				fmt.Println("-> Hilo de ayuda a aliados PAUSADO.")
				<-pausar // Espera aquí hasta recibir la señal de reanudar (false)
				fmt.Println("-> Hilo de ayuda a aliados REANUDADO.")
			}
		default:
			bitmap := robotgo.CaptureScreen()
			pantallaImg := robotgo.ToImage(bitmap)

			if GUARDAR_PRIMERA_CAPTURA_AYUDA {
				err := imgo.Save("primera_captura.png", pantallaImg)
				if err != nil {
					fmt.Println("Error al guardar la captura de pantalla :", err)
				} else {
					fmt.Println("Captura de pantalla guardada en 'primera_captura.png'.")
				}
				GUARDAR_PRIMERA_CAPTURA_AYUDA = false
			}

			// Llamamos a la nueva función con todos los parámetros.
			pt := buscarIcono(pantallaImg, iconoImg, AREA_BUSQUEDA_AYUDA, TOLERANCIA_COLOR, TOLERANCIA_PIXEL_PORCENTAJE)

			if pt.X != -1 {
				fmt.Printf("¡Icono de ayuda encontrado en (%d, %d)! Haciendo clic.\n", pt.X, pt.Y)
				robotgo.Move(pt.X, pt.Y)
				robotgo.Click()
				time.Sleep(3 * time.Second)
			} else {
				if DEBUG_MODE {
					fmt.Println("No se encontró el icono de ayuda en esta iteración.")
				}
				time.Sleep(1 * time.Second)
			}
		}
	}
}

func buscarReunion(wg *sync.WaitGroup, done <-chan bool, pausarAyuda chan bool) {
	defer wg.Done()
	fmt.Println("-> Hilo de reunión INICIADO.")
	iconoReunionImg, _ := imgo.Read(iconoReunionFile)
	iconoMasVerdeImg, _ := imgo.Read(iconoMasVerdeFile)
	iconoPartirImg, _ := imgo.Read(iconoPartirFile)

	clientOCR := gosseract.NewClient()
	defer clientOCR.Close()
	clientOCR.SetLanguage("eng")

	bIconoReunion := iconoReunionImg.Bounds()
	centroIconoReunionX := bIconoReunion.Dx() / 2
	centroIconoReunionY := bIconoReunion.Dy() / 2

	bIconoVerde := iconoMasVerdeImg.Bounds()
	centroIconoVerdeX := bIconoVerde.Dx() / 2
	centroIconoVerdeY := bIconoVerde.Dy() / 2

	bIconoPartir := iconoPartirImg.Bounds()
	centroIconoPartirX := bIconoPartir.Dx() / 2
	centroIconoPartirY := bIconoPartir.Dy() / 2

	for {
		select {
		case <-done:
			fmt.Println("-> Hilo de reunión DETENIDO.")
			return
		default:
			bitmap := robotgo.CaptureScreen()
			pantallaImg := robotgo.ToImage(bitmap)
			if GUARDAR_PRIMERA_CAPTURA_REUNION {
				imgo.Save("primera_captura_reunion.png", pantallaImg)
				GUARDAR_PRIMERA_CAPTURA_REUNION = false
			}
			pt := buscarIcono(pantallaImg, iconoReunionImg.(image.Image), AREA_BUSQUEDA_REUNION, TOLERANCIA_COLOR_REUNION, TOLERANCIA_PIXEL_REUNION)

			if pt.X != -1 {
				rectOCR := AREA_OCR_REUNION
				imgContadorCBitmap := robotgo.CaptureScreen(rectOCR.Min.X, rectOCR.Min.Y, rectOCR.Dx(), rectOCR.Dy())

				// Convertimos la captura a una imagen estándar de Go.
				imgContadorOriginal := robotgo.ToImage(imgContadorCBitmap)

				// --- INICIO DEL PRE-PROCESADO DE IMAGEN (NUEVO ORDEN) ---

				// 1. --- ¡NUEVO ORDEN! Redimensionamos la imagen a color PRIMERO ---
				// Usamos el algoritmo Bicubic como sugeriste para un resultado más suave.
				nuevoAncho := uint(imgContadorOriginal.Bounds().Dx() * 4)
				nuevoAlto := uint(imgContadorOriginal.Bounds().Dy() * 4)
				imgRedimensionada := resize.Resize(nuevoAncho, nuevoAlto, imgContadorOriginal, resize.Bicubic)

				// 2. Ahora, binarizamos e invertimos la imagen ya redimensionada
				bounds := imgRedimensionada.Bounds()
				imgProcesada := image.NewGray(bounds)
				umbral := uint8(180) // Puedes empezar con 180 y ajustarlo si es necesario

				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						// Obtenemos el color de la imagen grande y lo convertimos a escala de grises
						grayColor := color.GrayModel.Convert(imgRedimensionada.At(x, y)).(color.Gray)

						// Binarizamos e Invertimos
						if grayColor.Y > umbral {
							imgProcesada.Set(x, y, color.Black) // Texto brillante -> Negro
						} else {
							imgProcesada.Set(x, y, color.White) // Fondo oscuro -> Blanco
						}
					}
				}

				// 3. Añadimos el borde blanco a la imagen ya procesada y grande
				padding := 20
				imgFinalBounds := image.Rect(0, 0, imgProcesada.Bounds().Dx()+padding*2, imgProcesada.Bounds().Dy()+padding*2)
				imgFinal := image.NewGray(imgFinalBounds)
				draw.Draw(imgFinal, imgFinal.Bounds(), image.White, image.Point{}, draw.Src)
				drawPoint := image.Point{X: padding, Y: padding}
				draw.Draw(imgFinal, imgProcesada.Bounds().Add(drawPoint), imgProcesada, image.Point{}, draw.Over)

				// Guardamos la imagen final para depurar y comparar
				if DEBUG_MODE {
					imgo.Save("contador_inicial_redimensionado.png", imgRedimensionada)
					imgo.Save("contador_final_para_ocr.png", imgFinal)
				}

				// --- FIN DEL PRE-PROCESADO ---

				// --- INICIO DE LA SOLUCIÓN FINAL CON VARIABLE DE ENTORNO ---

				// 1. Creamos y gestionamos el archivo temporal como antes
				tempFile, err := os.CreateTemp("", "ocr-*.png")
				if err != nil {
					fmt.Println("Error al crear el archivo temporal:", err)
					continue
				}
				tempFileName := tempFile.Name()

				err = png.Encode(tempFile, imgFinal)
				if err != nil {
					fmt.Println("Error al guardar la imagen en el archivo temporal:", err)
					tempFile.Close()
					continue
				}
				tempFile.Close()

				// 2. Configuramos el resto de opciones
				clientOCR.SetImage(tempFileName)
				clientOCR.SetWhitelist("0123456789:")
				clientOCR.SetPageSegMode(gosseract.PSM_SINGLE_LINE)

				// 3. Ejecutamos el OCR
				textoContador, err := clientOCR.Text()
				if err != nil {
					fmt.Println("Error de OCR al ejecutar .Text():", err)
				}

				// 4. --- ¡LIMPIEZA MANUAL! ---
				// Borramos el archivo explícitamente ahora que ya no lo necesitamos.
				os.Remove(tempFileName)

				// --- FIN DE LA SOLUCIÓN ---

				if esTextoDeContador(textoContador) {
					fmt.Printf("   - Icono de reunión encontrado y contador de tiempo detectado ('%s').\n", strings.TrimSpace(textoContador))
					fmt.Println("¡Secuencia de reunión iniciada!")
					pausarAyuda <- true

					clickX := pt.X + centroIconoReunionX
					clickY := pt.Y + centroIconoReunionY
					fmt.Printf("   - Clic en centro de icono de reunión (%d, %d).\n", clickX, clickY)
					robotgo.Move(clickX, clickY)
					robotgo.Click()

					time.Sleep(1 * time.Second)

					pantallaReuniones := robotgo.ToImage(robotgo.CaptureScreen())
					ptBotonVerde := buscarBotonVerdeEnTarjetas(pantallaReuniones, iconoMasVerdeImg.(image.Image))
					if ptBotonVerde.X != -1 {
						clickVerdeX := ptBotonVerde.X + centroIconoVerdeX
						clickVerdeY := ptBotonVerde.Y + centroIconoVerdeY
						fmt.Printf("   - Botón '+' verde encontrado. Clic en centro (%d, %d).\n", clickVerdeX, clickVerdeY)
						robotgo.Move(clickVerdeX, clickVerdeY)
						robotgo.Click()

						time.Sleep(1 * time.Second)

						pantallaPartir := robotgo.ToImage(robotgo.CaptureScreen())
						ptBotonPartir := buscarIcono(pantallaPartir, iconoPartirImg.(image.Image), AREA_BUSQUEDA_BOTON_PARTIR, TOLERANCIA_COLOR_REUNION, TOLERANCIA_PIXEL_REUNION)
						if ptBotonPartir.X != -1 {
							clickPartirX := ptBotonPartir.X + centroIconoPartirX
							clickPartirY := ptBotonPartir.Y + centroIconoPartirY
							fmt.Printf("      - Botón 'Partir' encontrado. Clic en centro (%d, %d).\n", clickPartirX, clickPartirY)
							robotgo.Move(clickPartirX, clickPartirY)
							robotgo.Click()
						} else {
							fmt.Println("      - No se encontró botón 'Partir'. Volviendo atrás.")
							robotgo.Move(2300, 1820)
							robotgo.Click()
							time.Sleep(1 * time.Second)
							robotgo.Move(2333, 826)
							robotgo.Click()
						}
					} else {
						fmt.Println("   - No se encontró un botón '+' verde. Haciendo clic en 'Atrás'.")
						robotgo.Move(1410, 2045)
						robotgo.Click()
					}

					pausarAyuda <- false
					fmt.Println("¡Secuencia de reunión finalizada!")
				} else {
					if DEBUG_MODE {
						fmt.Println("Icono de reunión encontrado, pero sin contador de tiempo. El texto OCR fue: ", strings.TrimSpace(textoContador))
					}
				}
			} else {
				if DEBUG_MODE {
					fmt.Println("No se encontró el icono de reunión.")
				}
			}
			time.Sleep(2 * time.Second)
		}
	}
}

// --- FUNCIÓN PRINCIPAL ---

func main() {
	fmt.Println("========================================")
	fmt.Println("   BOT PARA DARK WAR SURVIVAL (v9 - Secuencia 'Partir')")
	fmt.Println("========================================")
	fmt.Println("El bot comenzará en 5 segundos...")
	time.Sleep(5 * time.Second)

	var wg sync.WaitGroup
	done := make(chan bool)
	pausarAyudaChan := make(chan bool)

	wg.Add(1)
	go buscarYAyudarAliados(&wg, done, pausarAyudaChan)

	wg.Add(1)
	go buscarReunion(&wg, done, pausarAyudaChan)

	fmt.Println("\nEl bot está en ejecución con 2 hilos coordinados. Presiona la tecla ENTER para detenerlo.")
	fmt.Scanln()

	close(done)
	wg.Wait()

	fmt.Println("\n¡Rutina de automatización completada y detenida de forma segura!")
}
