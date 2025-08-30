# Project Overview

This project is a Go-based automation bot for the game "Dark War Survival". It uses GUI automation to perform repetitive tasks, helping the user to progress in the game.

**Main Technologies:**

*   **Go:** The core language for the bot's logic.
*   **robotgo:** A library for cross-platform GUI automation, used to control the mouse and capture the screen.
*   **gosseract:** A wrapper for the Tesseract OCR engine, used to read text from the screen.
*   **imgo, resize:** Libraries for image processing, used to find icons and prepare images for OCR.

**Architecture:**

The bot runs two main tasks concurrently as goroutines:

1.  **Helping Allies:** This task repeatedly searches for an "ally help" icon on the screen and clicks it.
2.  **Joining Meetings:** This task looks for a "meeting" icon. When found, it uses OCR to read a timer and then performs a sequence of clicks to join the meeting, including finding and clicking a green "plus" button and a "depart" button.

The bot includes a debug mode and saves screenshots for troubleshooting. It also has a mechanism to pause the "ally help" task while the "meeting" sequence is in progress to avoid conflicts.

# Building and Running

**Prerequisites:**

*   Go programming language.
*   Tesseract OCR engine installed and available in the system's PATH.

**Dependencies:**

The project uses the following Go modules:

*   `github.com/go-vgo/robotgo`
*   `github.com/nfnt/resize`
*   `github.com/otiai10/gosseract`
*   `github.com/vcaesar/imgo`

To install the dependencies, run:

```sh
go get github.com/go-vgo/robotgo
go get github.com/nfnt/resize
go get github.com/otiai10/gosseract
go get github.com/vcaesar/imgo
```

**Running the bot:**

To run the bot, execute the following command in the project's root directory:

```sh
go run main.go
```

The bot will start after a 5-second delay. To stop the bot, press the `ENTER` key in the terminal.

# Development Conventions

*   **Configuration:** The bot's behavior is configured through constants at the beginning of the `main.go` file. These include settings for debug mode, image file paths, color and pixel tolerances for icon searching, and screen areas to search within.
*   **Concurrency:** The main tasks are implemented as separate functions (`buscarYAyudarAliados` and `buscarReunion`) and run as goroutines. A `sync.WaitGroup` is used to ensure a graceful shutdown.
*   **Pausing Tasks:** A channel (`pausarAyudaChan`) is used to coordinate the two tasks, allowing the "meeting" task to pause the "ally help" task to prevent interference.
*   **Image-based Recognition:** The bot relies on finding specific icons on the screen. The icon images are stored in the `resources` directory. The search logic allows for color and pixel tolerance to make the matching more robust.
*   **OCR:** The `gosseract` library is used to read text from the screen, specifically to detect timers for meetings. The image is pre-processed (resized, binarized, inverted) to improve OCR accuracy.
*   **Debugging:** A `DEBUG_MODE` constant can be enabled to print additional information to the console. The bot also saves screenshots (`primera_captura.png`, `primera_captura_reunion.png`, etc.) to help with debugging image recognition issues.
