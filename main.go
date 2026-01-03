package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strconv"
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

var AREA_BUSQUEDA_AYUDA = image.Rect(2290, 1545, 2310, 1545+20)

// --- CONFIGURACIÓN TAREA: REUNIÓN ---
const iconoReunionFile = "resources/reunion_icono.png"
const iconoMasVerdeFile = "resources/mas_verde_icono.png"
const TOLERANCIA_COLOR_REUNION uint32 = 30000   // Puedes ajustar esta tolerancia si es necesario
const TOLERANCIA_PIXEL_REUNION = 0.05           // Y también este porcentaje
const TOLERANCIA_COLOR_MAS_VERDE uint32 = 20000 // Puedes ajustar esta tolerancia si es necesario
const TOLERANCIA_PIXEL_MAS_VERDE = 0.05         // Y también este porcentaje

var AREA_BUSQUEDA_REUNION = image.Rect(1490, 1735, 1770, 1755)
var AREA_OCR_PARTIR = image.Rect(1836, 1355, 2015, 1420)
var AREA_OCR_REUNION = image.Rect(2380, 1293, 2490, 1323)

// --- CONFIGURACIÓN DE BÚSQUEDA OPTIMIZADA DE BOTÓN VERDE ---
const (
	ALTO_TARJETA_REUNION     = 486
	ESPACIO_TARJETA_REUNION  = 30
	NUMERO_TARJETAS_VISIBLES = 3
)

var AREA_BUSQUEDA_BOTON_VERDE_INICIAL = image.Rect(2300, 420, 2320, 440)

// --- CONFIGURACIÓN DE LECTURA DE REUNIONES AUTOMÁTICAS ---
const (
	CARD_HEIGHT           = 275
	CARD_SPACING          = 25
	FIRST_CARD_Y          = 615
	CARD_TITLE_X_START    = 1710
	CARD_TITLE_X_END      = 2365
	CARD_TITLE_HEIGHT     = 56
	CARD_REWARD_HEIGHT    = 32
	NUM_CARDS_TO_CHECK    = 4
	REWARD_TEXT_PREFIX    = "Recompensas por unirse a la reunión:"
	REWARD_TEXT_SEPARATOR = "/"
)

// --- CONFIGURACIÓN DE ACCIONES INICIALES ---
const (
	ANCHO_PANTALLA            = 3840
	Y_INICIAL_OPCIONES_EVENTO = 175
	ALTO_OPCION_EVENTO        = 410
	ANCHO_OPCION_EVENTO       = 1090
	ESPACIO_OPCION_EVENTO     = 27
)

var AREA_BUSQUEDA_PANTALLA_INICIAL = image.Rect(2300, 2080, 2520, 2150)
var POS_EVENTOS_REGULARES = image.Point{X: 1384, Y: 1484}
var POS_CONFIRMAR_DESABILITAR_REUNIONES_AUTOMATICAS = image.Point{X: 2130, Y: 1350}

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

// normalizarTexto convierte un texto a minúsculas y le quita los acentos.
func normalizarTexto(s string) string {
	lower := strings.ToLower(s)
	replacer := strings.NewReplacer(
		"á", "a",
		"é", "e",
		"í", "i",
		"ó", "o",
		"ú", "u",
	)
	return replacer.Replace(lower)
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

	clientOCR := gosseract.NewClient()
	defer clientOCR.Close()
	clientOCR.SetLanguage("eng")

	bIconoReunion := iconoReunionImg.Bounds()
	centroIconoReunionX := bIconoReunion.Dx() / 2
	centroIconoReunionY := bIconoReunion.Dy() / 2

	bIconoVerde := iconoMasVerdeImg.Bounds()
	centroIconoVerdeX := bIconoVerde.Dx() / 2
	centroIconoVerdeY := bIconoVerde.Dy() / 2

	for {
		select {
		case <-done:
			fmt.Println("-> Hilo de reunión DETENIDO.")
			return
		default:
			bitmap := robotgo.CaptureScreen()
			pantallaImg := robotgo.ToImage(bitmap)
			if DEBUG_MODE {
				imgo.Save("primera_captura_reunion.png", pantallaImg)
			}
			pt := buscarIcono(pantallaImg, iconoReunionImg.(image.Image), AREA_BUSQUEDA_REUNION, TOLERANCIA_COLOR_REUNION, TOLERANCIA_PIXEL_REUNION)

			if pt.X != -1 {
				rectOCR := image.Rect(pt.X-40, pt.Y+iconoReunionImg.Bounds().Dy()+10, pt.X+iconoReunionImg.Bounds().Dx()+40, pt.Y+iconoReunionImg.Bounds().Dy()+40)
				imgContadorCBitmap := robotgo.CaptureScreen(rectOCR.Min.X, rectOCR.Min.Y, rectOCR.Dx(), rectOCR.Dy())

				// Convertimos la captura a una imagen estándar de Go.
				imgContadorOriginal := robotgo.ToImage(imgContadorCBitmap)
				if DEBUG_MODE {
					imgo.Save("captura_contador.png", imgContadorOriginal)
				}

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

						// --- Lógica OCR para el botón 'Partir' (Búsqueda en 3 posiciones) ---
						encontradoPartir := false
						for intento := 0; intento < 3; intento++ {
							offsetY := intento * 120
							rectPartir := image.Rect(
								AREA_OCR_PARTIR.Min.X,
								AREA_OCR_PARTIR.Min.Y+offsetY,
								AREA_OCR_PARTIR.Max.X,
								AREA_OCR_PARTIR.Max.Y+offsetY,
							)

							imgPartirBitmap := robotgo.CaptureScreen(rectPartir.Min.X, rectPartir.Min.Y, rectPartir.Dx(), rectPartir.Dy())
							imgPartirOriginal := robotgo.ToImage(imgPartirBitmap)

							if DEBUG_MODE {
								imgo.Save(fmt.Sprintf("boton_partir_intento_%d.png", intento), imgPartirOriginal)
							}

							// Pre-procesamiento para el botón 'Partir'
							nAnchoP := uint(imgPartirOriginal.Bounds().Dx() * 4)
							nAltoP := uint(imgPartirOriginal.Bounds().Dy() * 4)
							imgPartirResized := resize.Resize(nAnchoP, nAltoP, imgPartirOriginal, resize.Bicubic)

							boundsP := imgPartirResized.Bounds()
							imgPartirProcesada := image.NewGray(boundsP)
							for y := boundsP.Min.Y; y < boundsP.Max.Y; y++ {
								for x := boundsP.Min.X; x < boundsP.Max.X; x++ {
									grayColor := color.GrayModel.Convert(imgPartirResized.At(x, y)).(color.Gray)
									if grayColor.Y > 180 {
										imgPartirProcesada.Set(x, y, color.Black)
									} else {
										imgPartirProcesada.Set(x, y, color.White)
									}
								}
							}

							clientOCR.SetImageFromBytes(imgToBytes(imgPartirProcesada))
							clientOCR.SetWhitelist("") // Reset whitelist for text
							clientOCR.SetLanguage("spa")
							textoPartirRaw, _ := clientOCR.Text()
							textoPartir := normalizarTexto(strings.TrimSpace(textoPartirRaw))

							if strings.Contains(textoPartir, "partir") {
								clickPartirX := rectPartir.Min.X + rectPartir.Dx()/2
								clickPartirY := rectPartir.Min.Y + rectPartir.Dy()/2

								// --- Comprobar si el botón está habilitado (Color en 0,0) ---
								r, g, b, _ := imgPartirOriginal.At(0, 0).RGBA()
								// En robotgo/image, RGBA() devuelve valores de 16 bits.
								// Para simplificar, comparamos si el componente Verde es dominante.
								esVerde := g > r && g > b

								if esVerde {
									fmt.Printf("      - Botón 'Partir' habilitado (VERDE) detectado en intento %d. Haciendo clic en (%d, %d).\n", intento+1, clickPartirX, clickPartirY)
									robotgo.Move(clickPartirX, clickPartirY)
								} else {
									fmt.Printf("      - Botón 'Partir' deshabilitado (GRIS) detectado en intento %d. Saliendo...\n", intento+1)
									robotgo.Move(clickPartirX, 0)
								}

								robotgo.Click()
								encontradoPartir = true
								break
							} else if DEBUG_MODE {
								fmt.Printf("      - Intento %d: No se detectó 'Partir' (OCR: '%s').\n", intento+1, textoPartir)
							}
						}

						if !encontradoPartir {
							fmt.Println("      - No se detectó el texto 'Partir' en ninguna de las 3 posiciones. Volviendo atrás.")
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

// --- LÓGICA DE PREPARACIÓN ---

// clickOpcionEvento calcula y hace clic en una opción de la lista de eventos.
func clickOpcionEvento(opcion int) {
	// La X se calcula como el centro horizontal de la pantalla.
	clickX := ANCHO_PANTALLA / 2
	// La Y se calcula según la fórmula proporcionada.
	clickY := Y_INICIAL_OPCIONES_EVENTO + (ALTO_OPCION_EVENTO+ESPACIO_OPCION_EVENTO)*opcion + (ALTO_OPCION_EVENTO / 2)

	fmt.Printf("Haciendo clic en la opción de evento %d en (%d, %d).\n", opcion+1, clickX, clickY)
	robotgo.Move(clickX, clickY)
	robotgo.Click()
}

// Deshabilita las reuniones automáticas
func deshabilitarReunionesAutomaticas() {
	fmt.Println("Iniciando secuencia de preparación de reuniones...")

	// 1. Clic en 'Eventos Regulares'
	fmt.Println("Paso 1: Clic en 'Eventos Regulares' en (", POS_EVENTOS_REGULARES.X, ",", POS_EVENTOS_REGULARES.Y, ").")
	robotgo.Move(POS_EVENTOS_REGULARES.X, POS_EVENTOS_REGULARES.Y)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 2. Clic en la segunda opción ('Reunión de Zombies')
	// Usamos el índice 1 para la segunda opción (0-indexed).
	fmt.Println("Paso 2: Clic en la segunda opción del menú de eventos.")
	clickOpcionEvento(1)
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 3. Clic en 'Reuniones Automáticas'
	fmt.Println("Paso 3: Clic en 'Reuniones Automáticas' en (1920, 2040).")
	robotgo.Move(1920, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 4. Clic en el botón 'Cerrar' (Reuniones automáticas)
	fmt.Println("Paso 4: Clic en 'Cerrar' en el centro de (1588, 1572) - (1785, 1640).")
	robotgo.Move((1588+1785)/2, (1572+1640)/2)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que aparezca la confirmación

	// 4b. Clic en el botón de confirmación
	fmt.Println("Paso 4b: Clic en el botón de confirmación en (", POS_CONFIRMAR_DESABILITAR_REUNIONES_AUTOMATICAS.X, ",", POS_CONFIRMAR_DESABILITAR_REUNIONES_AUTOMATICAS.Y, ").")
	robotgo.Move(POS_CONFIRMAR_DESABILITAR_REUNIONES_AUTOMATICAS.X, POS_CONFIRMAR_DESABILITAR_REUNIONES_AUTOMATICAS.Y)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 5. Clic en la 'X' (cerrar ventana)
	fmt.Println("Paso 5: Clic en la 'X' para cerrar la ventana (2345, 450).")
	robotgo.Move(2345, 450)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 6. Clic en el botón 'Atrás' (flecha a la izquierda)
	fmt.Println("Paso 6: Clic en 'Atrás' para salir de 'Asedio Al Gigante' (1400, 2040).")
	robotgo.Move(1400, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 7. Clic en el botón 'Atrás' (flecha a la izquierda)
	fmt.Println("Paso 7: Clic en 'Atrás' para salir de 'Eventos Regulares' (1400, 2040).")
	robotgo.Move(1400, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	fmt.Println("Secuencia de preparación de reuniones finalizada.")
}

// Habilita las reuniones automáticas
func habilitarReunionesAutomaticas() {
	fmt.Println("Iniciando secuencia de preparación de reuniones...")

	// 1. Clic en 'Eventos Regulares'
	fmt.Println("Paso 1: Clic en 'Eventos Regulares' en (", POS_EVENTOS_REGULARES.X, ",", POS_EVENTOS_REGULARES.Y, ").")
	robotgo.Move(POS_EVENTOS_REGULARES.X, POS_EVENTOS_REGULARES.Y)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 2. Clic en la segunda opción ('Reunión de Zombies')
	// Usamos el índice 1 para la segunda opción (0-indexed).
	fmt.Println("Paso 2: Clic en la segunda opción del menú de eventos.")
	clickOpcionEvento(1)
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 3. Clic en 'Reuniones Automáticas'
	fmt.Println("Paso 3: Clic en 'Reuniones Automáticas' en (1920, 2040).")
	robotgo.Move(1920, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 4. Clic en el botón 'Abrir' (Reuniones automáticas)
	fmt.Println("Paso 4: Clic en 'Abrir' en el centro de (2056, 1572) - (2221, 1640).")
	robotgo.Move((2221+2056)/2, (1640+1572)/2)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 5. Clic en la 'X' (cerrar ventana)
	fmt.Println("Paso 5: Clic en la 'X' para cerrar la ventana (2345, 450).")
	robotgo.Move(2345, 450)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 6. Clic en el botón 'Atrás' (flecha a la izquierda)
	fmt.Println("Paso 6: Clic en 'Atrás' para salir de 'Asedio Al Gigante' (1400, 2040).")
	robotgo.Move(1400, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	// 7. Clic en el botón 'Atrás' (flecha a la izquierda)
	fmt.Println("Paso 7: Clic en 'Atrás' para salir de 'Eventos Regulares' (1400, 2040).")
	robotgo.Move(1400, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second) // Pausa para que la UI responda

	fmt.Println("Secuencia de preparación de reuniones finalizada.")
}

// obtenerReunionesPendientes busca el número de reuniones pendientes para un tipo específico.
// obtenerReunionesPendientes escanea todas las tarjetas visibles y devuelve un mapa con las reuniones pendientes de cada tipo.
func obtenerReunionesPendientes() map[string]int {
	fmt.Println("Iniciando obtención unificada de reuniones pendientes...")
	resultados := map[string]int{
		"Zombi Gigante":         -1,
		"Zombi Momia [Gigante]": -1,
		"Zombis de la tundra":   -1,
		"Caza con Victor":       -1,
	}

	tiposSoportados := []string{
		"Zombi Gigante",
		"Zombi Momia [Gigante]",
		"Zombis de la tundra",
		"Caza con Victor",
	}

	// 1. Clic en 'Eventos Regulares'
	robotgo.Move(POS_EVENTOS_REGULARES.X, POS_EVENTOS_REGULARES.Y)
	robotgo.Click()
	time.Sleep(1 * time.Second)

	// 2. Clic en la segunda opción ('Reunión de Zombies')
	clickOpcionEvento(1)
	time.Sleep(1 * time.Second)

	// 3. Clic en 'Reuniones Automáticas'
	robotgo.Move(1920, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second)

	// --- Lógica de lectura de tarjetas ---
	clientOCR := gosseract.NewClient()
	defer clientOCR.Close()
	clientOCR.SetLanguage("spa")

	for i := 0; i < NUM_CARDS_TO_CHECK; i++ {
		cardY := FIRST_CARD_Y + i*(CARD_HEIGHT+CARD_SPACING)

		// --- Leer el título de la tarjeta ---
		titleRect := image.Rect(CARD_TITLE_X_START, cardY, CARD_TITLE_X_END, cardY+CARD_TITLE_HEIGHT)
		imgTituloBitmap := robotgo.CaptureScreen(titleRect.Min.X, titleRect.Min.Y, titleRect.Dx(), titleRect.Dy())
		imgTitulo := robotgo.ToImage(imgTituloBitmap)

		// Pre-procesamiento
		nuevoAncho := uint(imgTitulo.Bounds().Dx() * 2)
		nuevoAlto := uint(imgTitulo.Bounds().Dy() * 2)
		imgRedimensionada := resize.Resize(nuevoAncho, nuevoAlto, imgTitulo, resize.Bicubic)
		bounds := imgRedimensionada.Bounds()
		imgProcesada := image.NewGray(bounds)
		umbral := uint8(210)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				grayColor := color.GrayModel.Convert(imgRedimensionada.At(x, y)).(color.Gray)
				if grayColor.Y > umbral {
					imgProcesada.Set(x, y, color.Black)
				} else {
					imgProcesada.Set(x, y, color.White)
				}
			}
		}

		// OCR al título
		clientOCR.SetImageFromBytes(imgToBytes(imgProcesada))
		textoTituloRaw, err := clientOCR.Text()
		if err != nil {
			fmt.Printf("Error de OCR en el título de la tarjeta %d: %v\n", i, err)
			continue
		}
		textoTitulo := normalizarTexto(strings.TrimSpace(textoTituloRaw))

		if DEBUG_MODE {
			fmt.Printf("Tarjeta %d: Título detectado: '%s'\n", i, textoTitulo)
		}

		// Identificar qué tipo de reunión es
		tipoEncontrado := ""
		for _, t := range tiposSoportados {
			if strings.Contains(textoTitulo, normalizarTexto(t)) {
				tipoEncontrado = t
				break
			}
		}

		if tipoEncontrado != "" {
			if DEBUG_MODE {
				fmt.Printf("   -> Coincidencia con: %s\n", tipoEncontrado)
			}

			// --- Leer el texto de la recompensa ---
			rewardY := cardY + CARD_TITLE_HEIGHT
			rewardRect := image.Rect(CARD_TITLE_X_START, rewardY, CARD_TITLE_X_END, rewardY+CARD_REWARD_HEIGHT)
			imgRecompensaBitmap := robotgo.CaptureScreen(rewardRect.Min.X, rewardRect.Min.Y, rewardRect.Dx(), rewardRect.Dy())
			imgRecompensa := robotgo.ToImage(imgRecompensaBitmap)

			// Pre-procesamiento similar
			nuevoAnchoR := uint(imgRecompensa.Bounds().Dx() * 2)
			nuevoAltoR := uint(imgRecompensa.Bounds().Dy() * 2)
			imgRedimensionadaR := resize.Resize(nuevoAnchoR, nuevoAltoR, imgRecompensa, resize.Bicubic)
			boundsR := imgRedimensionadaR.Bounds()
			imgProcesadaR := image.NewGray(boundsR)
			for y := boundsR.Min.Y; y < boundsR.Max.Y; y++ {
				for x := boundsR.Min.X; x < boundsR.Max.X; x++ {
					grayColor := color.GrayModel.Convert(imgRedimensionadaR.At(x, y)).(color.Gray)
					if grayColor.Y > 160 {
						imgProcesadaR.Set(x, y, color.Black)
					} else {
						imgProcesadaR.Set(x, y, color.White)
					}
				}
			}

			// OCR al texto de recompensa
			clientOCR.SetImageFromBytes(imgToBytes(imgProcesadaR))
			textoRecompensa, err := clientOCR.Text()
			if err == nil {
				textoRecompensa = strings.TrimSpace(textoRecompensa)
				// Parsear "N/M"
				parts := strings.Split(textoRecompensa, REWARD_TEXT_PREFIX)
				if len(parts) > 1 {
					numerosStr := strings.TrimSpace(parts[1])
					valores := strings.Split(numerosStr, REWARD_TEXT_SEPARATOR)
					if len(valores) == 2 {
						n, errN := strconv.Atoi(strings.TrimSpace(valores[0]))
						m, errM := strconv.Atoi(strings.TrimSpace(valores[1]))
						if errN == nil && errM == nil {
							resultados[tipoEncontrado] = m - n
							fmt.Printf("   -> %s: %d reuniones pendientes (%d/%d).\n", tipoEncontrado, m-n, n, m)
						}
					}
				}
			}
		}
	}

	// --- Limpieza ---
	fmt.Println("Regresando a la pantalla del mundo (Limpieza)...")
	robotgo.Move(2345, 450)
	robotgo.Click()
	time.Sleep(1 * time.Second)

	robotgo.Move(1400, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second)

	robotgo.Move(1400, 2040)
	robotgo.Click()
	time.Sleep(1 * time.Second)

	return resultados
}

// irAlMundo verifica si el bot está en la pantalla "Mundo" o "Refugio".
// Si está en "Refugio", hace clic para ir al "Mundo".
func irAlMundo() {
	fmt.Println("Navegando al Mundo...")

	// 1. Capturar el área específica
	rect := AREA_BUSQUEDA_PANTALLA_INICIAL
	imgBitmap := robotgo.CaptureScreen(rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy())
	imgOriginal := robotgo.ToImage(imgBitmap)

	// 2. Pre-procesamiento
	nuevoAncho := uint(imgOriginal.Bounds().Dx() * 4)
	nuevoAlto := uint(imgOriginal.Bounds().Dy() * 4)
	imgRedimensionada := resize.Resize(nuevoAncho, nuevoAlto, imgOriginal, resize.Bicubic)

	bounds := imgRedimensionada.Bounds()
	imgProcesada := image.NewGray(bounds)
	umbral := uint8(180)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			grayColor := color.GrayModel.Convert(imgRedimensionada.At(x, y)).(color.Gray)
			if grayColor.Y > umbral {
				imgProcesada.Set(x, y, color.Black)
			} else {
				imgProcesada.Set(x, y, color.White)
			}
		}
	}

	// 3. OCR
	clientOCR := gosseract.NewClient()
	defer clientOCR.Close()
	clientOCR.SetLanguage("spa")
	clientOCR.SetImageFromBytes(imgToBytes(imgProcesada))

	texto, err := clientOCR.Text()
	if err != nil {
		fmt.Printf("Error de OCR al identificar pantalla: %v\n", err)
		os.Exit(1)
	}

	textoNormalizado := normalizarTexto(strings.TrimSpace(texto))

	if DEBUG_MODE {
		fmt.Printf("Texto detectado en el botón de cambio de pantalla: '%s'\n", textoNormalizado)
		imgo.Save("identificacion_pantalla_redimensionada.png", imgRedimensionada)
		imgo.Save("identificacion_pantalla_ocr.png", imgProcesada)
	}

	if strings.Contains(textoNormalizado, "mundo") {
		// Estamos en el Refugio, el botón dice "Mundo"
		fmt.Println("Estamos en REFUGIO. Navegando a MUNDO...")
		// Calculamos el centro del área para hacer clic
		clickX := rect.Min.X + (rect.Dx() / 2)
		clickY := rect.Min.Y + (rect.Dy() / 2)
		robotgo.Move(clickX, clickY)
		robotgo.Click()
		time.Sleep(2 * time.Second) // Espera para que cargue el mundo
	} else if strings.Contains(textoNormalizado, "refugio") {
		// Ya estamos en el Mundo, el botón dice "Refugio"
		fmt.Println("Ya estamos en MUNDO. No es necesario navegar.")
	} else {
		fmt.Printf("Error: No se ha podido identificar la pantalla inicial (Texto detectado: '%s').\n", textoNormalizado)
		fmt.Println("El bot solo puede iniciarse desde las pantallas 'Mundo' o 'Refugio'.")
		os.Exit(1)
	}
}

// imgToBytes es una función helper para convertir image.Image a []byte para gosseract
func imgToBytes(img image.Image) []byte {
	tempFile, err := os.CreateTemp("", "ocr-helper-*.png")
	if err != nil {
		return nil
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	png.Encode(tempFile, img)
	bytes, err := os.ReadFile(tempFile.Name())
	if err != nil {
		return nil
	}
	return bytes
}

// --- FUNCIÓN PRINCIPAL ---

func main() {
	fmt.Println("========================================")
	fmt.Println("   BOT PARA DARK WAR SURVIVAL (v9 - Secuencia 'Partir')")
	fmt.Println("========================================")
	fmt.Println("El bot comenzará en 5 segundos...")
	time.Sleep(5 * time.Second)

	irAlMundo()

	reuniones := obtenerReunionesPendientes()

	fmt.Println("\n--- Resumen de Reuniones ---")
	for tipo, pendientes := range reuniones {
		if pendientes == -1 {
			fmt.Printf("- %s: No encontrado en esta temporada.\n", tipo)
		} else {
			fmt.Printf("- %s: %d pendientes.\n", tipo, pendientes)
		}
	}
	fmt.Println("----------------------------")

	deshabilitarReunionesAutomaticas()

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
