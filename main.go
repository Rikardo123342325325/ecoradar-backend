package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

// ==========================================
// 1. ESTRUCTURAS ORIGINALES
// ==========================================

// Estructura para LEER reportes (GET)
type Reporte struct {
	ID          int     `json:"id_reportes"`
	Titulo      string  `json:"titulo"`
	Descripcion string  `json:"descripcion"`
	Latitud     float64 `json:"latitud"`
	Longitud    float64 `json:"longitud"`
	Estado      string  `json:"estado"`
	Fotografia  string  `json:"fotografia"`
	Categoria   string  `json:"categoria"`
	Fecha       string  `json:"fecha"`
}

// Estructura para CREAR reportes (POST)
type NuevoReporte struct {
	IdUsuario    int     `form:"id_usuario"`
	IdCategorias int     `form:"id_categorias"`
	Titulo       string  `form:"titulo"`
	Descripcion  string  `form:"descripcion"`
	Latitud      float64 `form:"latitud"`
	Longitud     float64 `form:"longitud"`
}

// Estructura para recibir los datos del Login
type LoginData struct {
	Correo     string `json:"correo"`
	Contrasena string `json:"contrasena"`
}

type EstadoUpdate struct {
	NuevoEstado string `json:"nuevo_estado"`
}

// ==========================================
// 2. NUEVAS ESTRUCTURAS (PANTALLAS DASHBOARD)
// ==========================================

type Brigada struct {
	ID           int    `json:"id"`
	Nombre       string `json:"nombre"`
	Especialidad string `json:"especialidad"`
	Ubicacion    string `json:"ubicacion"`
	Tareas       int    `json:"tareas"`
	Estado       string `json:"estado"`
	Color        string `json:"color"`
}

type Alerta struct {
	ID          int    `json:"id"`
	Titulo      string `json:"titulo"`
	Descripcion string `json:"descripcion"`
	Tipo        string `json:"tipo"`
}

type Zona struct {
	ID           int    `json:"id"`
	Nombre       string `json:"nombre"`
	Descripcion  string `json:"descripcion"`
	EstadoActual string `json:"estado_actual"`
	Color        string `json:"color"`
}

type UsuarioAdmin struct {
	ID     int    `json:"id"`
	Nombre string `json:"nombre"`
	Correo string `json:"correo"`
	Rol    string `json:"rol"`
	Estado string `json:"estado"`
}

// ==========================================
// 3. MOTOR DEL SERVIDOR (MAIN)
// ==========================================

func main() {
	// 1. Conexión a la BD en la Nube (Aiven)
	dsn := "avnadmin:AVNS_z1CZB540pqCrdxWXGYB@tcp(ecoradar-bd-ricardoruiz-3eaf.k.aivencloud.com:27416)/defaultdb?parseTime=true&tls=skip-verify"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	// Nos aseguramos de cerrar la BD al apagar el servidor
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("Error conectando a la BD:", err)
	}

	// =========================================================================
	// ✨ MAGIA PARA EVITAR QUE AIVEN CIERRE LA CONEXIÓN (POOL DE CONEXIONES)
	// =========================================================================

	// SetConnMaxLifetime establece la cantidad máxima de tiempo que una conexión puede ser reutilizada.
	// Aiven suele cerrar las inactivas a los 5 minutos. Si le decimos a Go que las recicle cada 1 minuto,
	// evitamos el error "invalid connection" o "connection reset by peer".
	db.SetConnMaxLifetime(time.Minute * 1)

	// SetMaxOpenConns establece el número máximo de conexiones abiertas a la base de datos.
	// Ayuda a no saturar los límites de tu plan gratuito en Aiven.
	db.SetMaxOpenConns(10)

	// SetMaxIdleConns establece el número máximo de conexiones inactivas en el pool.
	// Mantiene conexiones "calientes" para que la API responda rápido sin reconectar siempre.
	db.SetMaxIdleConns(10)

	// SetConnMaxIdleTime cierra las conexiones que llevan mucho tiempo sin hacer nada.
	db.SetConnMaxIdleTime(time.Minute * 1)

	// =========================================================================

	// 2. Crear la carpeta "uploads" si no existe para guardar las fotos ahí
	os.MkdirAll("uploads", os.ModePerm)

	r := gin.Default()

	// --- MAGIA DE CORS (PASAPORTE UNIVERSAL) ---
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	// -------------------------------------------

	// 3. Exponer la carpeta de fotos
	r.Static("/uploads", "./uploads")

	// RUTA GET: Verificar estado
	r.GET("/api/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"mensaje": "API en línea y BD conectada establemente"})
	})

	// RUTA GET: Obtener todos los reportes con su Categoría
	r.GET("/api/reportes", func(c *gin.Context) {
		query := `
			SELECT r.id_reportes, r.titulo, r.descripcion, r.latitud, r.longitud, 
				   IFNULL(r.estado, 'Pendiente'), IFNULL(r.fotografia, ''), IFNULL(cat.nombre_categorias, 'General'),
				   IFNULL(r.fecha_creacion, NOW()) -- <--- ESTO ES LO NUEVO
			FROM tbreportes r
			LEFT JOIN tbcategorias cat ON r.id_categorias = cat.id_categorias
		`
		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var reportes []Reporte
		for rows.Next() {
			var rep Reporte
			// Agrégale &rep.Fecha justo antes del paréntesis de cierre:
			if err := rows.Scan(&rep.ID, &rep.Titulo, &rep.Descripcion, &rep.Latitud, &rep.Longitud, &rep.Estado, &rep.Fotografia, &rep.Categoria, &rep.Fecha); err != nil {
				log.Println("Error leyendo fila:", err)
				continue
			}
			reportes = append(reportes, rep)
		}
		c.JSON(http.StatusOK, reportes)
	})

	// RUTA POST: Crear un nuevo reporte (Subiendo la foto a Cloudinary)
	r.POST("/api/reportes", func(c *gin.Context) {
		// Recibimos los datos del formulario (FormData)
		// Convertimos a los tipos correctos para evitar errores en MySQL
		idUsuario, _ := strconv.Atoi(c.PostForm("id_usuario"))
		idCategorias, _ := strconv.Atoi(c.PostForm("id_categorias"))
		titulo := c.PostForm("titulo")
		descripcion := c.PostForm("descripcion")
		latitud, _ := strconv.ParseFloat(c.PostForm("latitud"), 64)
		longitud, _ := strconv.ParseFloat(c.PostForm("longitud"), 64)

		// --- FOTO: OPCIONAL ---
		// Si viene foto la subimos a Cloudinary, si no, guardamos cadena vacía
		urlFoto := ""
		formFile, errFoto := c.FormFile("fotografia")
		if errFoto == nil {
			// Hay foto — abrirla y subirla a Cloudinary
			openedFile, errOpen := formFile.Open()
			if errOpen != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Error al leer el archivo"})
				return
			}
			defer openedFile.Close()

			cld, errCld := cloudinary.NewFromURL(os.Getenv("CLOUDINARY_URL"))
			if errCld != nil {
				log.Println("❌ Error al conectar con Cloudinary:", errCld)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Error de configuración de Cloudinary"})
				return
			}
			ctx := context.Background()

			resp, errUpload := cld.Upload.Upload(ctx, openedFile, uploader.UploadParams{
				Folder: "ecoradar",
			})
			if errUpload != nil {
				log.Println("❌ Error al subir a Cloudinary:", errUpload)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Falló la subida de imagen a la nube"})
				return
			}
			urlFoto = resp.SecureURL
			log.Println("✅ Foto subida exitosamente a:", urlFoto)
		} else {
			log.Println("ℹ️ Reporte sin fotografía, se guarda sin imagen.")
		}

		// --- INSERTAR EN BD ---
		query := `
			INSERT INTO tbreportes (id_usuario, id_categorias, titulo, descripcion, latitud, longitud, fotografia, estado, fecha_creacion)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'Pendiente', NOW())
		`
		_, err := db.Exec(query, idUsuario, idCategorias, titulo, descripcion, latitud, longitud, urlFoto)
		if err != nil {
			log.Println("❌ Error al guardar en MySQL:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo guardar el reporte en la base de datos"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"mensaje": "Reporte creado exitosamente", "foto_url": urlFoto})
	})

	// RUTA PUT: Actualizar el estado
	r.PUT("/api/reportes/:id/estado", func(c *gin.Context) {
		id := c.Param("id")
		var datos EstadoUpdate
		if err := c.ShouldBindJSON(&datos); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Datos inválidos"})
			return
		}
		query := "UPDATE tbreportes SET estado = ? WHERE id_reportes = ?"
		_, err := db.Exec(query, datos.NuevoEstado, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"mensaje": "Estado actualizado correctamente"})
	})

	// RUTA POST: Validar Login de Administrador
	r.POST("/api/login", func(c *gin.Context) {
		var login LoginData
		if err := c.ShouldBindJSON(&login); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Datos inválidos"})
			return
		}

		var id int
		var nombre string

		query := "SELECT id_usuarios, usuarios_nombre FROM tbusuarios WHERE usuarios_correo = ? AND usuarios_contrasena = ?"

		err := db.QueryRow(query, login.Correo, login.Contrasena).Scan(&id, &nombre)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Correo o contraseña incorrectos"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"mensaje":    "Login exitoso",
			"token":      "eco_token_valido_xyz123",
			"usuario":    nombre,
			"id_usuario": id,
		})
	})

	// ==========================================
	// 4. NUEVAS RUTAS PANTALLAS ADMINISTRADOR
	// ==========================================

	// Obtener Brigadas
	r.GET("/api/brigadas", func(c *gin.Context) {
		rows, err := db.Query("SELECT id_brigada, nombre, especialidad, ubicacion_actual, tareas_activas, estado, color_borde FROM tbbrigadas")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()
		var lista []Brigada
		for rows.Next() {
			var b Brigada
			if err := rows.Scan(&b.ID, &b.Nombre, &b.Especialidad, &b.Ubicacion, &b.Tareas, &b.Estado, &b.Color); err == nil {
				lista = append(lista, b)
			}
		}
		c.JSON(http.StatusOK, lista)
	})

	// Obtener Alertas
	r.GET("/api/alertas", func(c *gin.Context) {
		rows, err := db.Query("SELECT id_alerta, titulo, descripcion, tipo FROM tbalertas ORDER BY fecha_generacion DESC")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()
		var lista []Alerta
		for rows.Next() {
			var a Alerta
			if err := rows.Scan(&a.ID, &a.Titulo, &a.Descripcion, &a.Tipo); err == nil {
				lista = append(lista, a)
			}
		}
		c.JSON(http.StatusOK, lista)
	})

	// Obtener Zonas
	r.GET("/api/zonas", func(c *gin.Context) {
		rows, err := db.Query("SELECT id_zona, nombre, descripcion, estado_actual, color_alerta FROM tbzonas")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()
		var lista []Zona
		for rows.Next() {
			var z Zona
			if err := rows.Scan(&z.ID, &z.Nombre, &z.Descripcion, &z.EstadoActual, &z.Color); err == nil {
				lista = append(lista, z)
			}
		}
		c.JSON(http.StatusOK, lista)
	})

	// Obtener Usuarios Administradores
	r.GET("/api/usuarios", func(c *gin.Context) {
		rows, err := db.Query("SELECT id_usuario, nombre, correo, rol, estado FROM tbusuarios_admin")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()
		var lista []UsuarioAdmin
		for rows.Next() {
			var u UsuarioAdmin
			if err := rows.Scan(&u.ID, &u.Nombre, &u.Correo, &u.Rol, &u.Estado); err == nil {
				lista = append(lista, u)
			}
		}
		c.JSON(http.StatusOK, lista)
	})
	// RUTA DE IA: Generar análisis inteligente
	r.POST("/api/ia/analisis", func(c *gin.Context) {
		var data []Reporte // Recibimos la lista de reportes actuales
		if err := c.ShouldBindJSON(&data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Datos inválidos"})
			return
		}

		// Convertimos los reportes a un texto simple para que la IA los lea
		contexto := "Analiza estos reportes ciudadanos de Villahermosa y dime: 1. Patrón detectado. 2. Zona más crítica. 3. Una recomendación estratégica:\n"
		for _, r := range data {
			contexto += "- " + r.Categoria + ": " + r.Descripcion + " en " + r.Titulo + "\n"
		}

		// Configuración de la petición a Google Gemini
		apiKey := os.Getenv("GEMINI_API_KEY")
		urlIA := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=" + apiKey

		payload := map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"parts": []map[string]string{
						{"text": contexto},
					},
				}, //
			},
		}

		jsonPayload, _ := json.Marshal(payload)
		resp, err := http.Post(urlIA, "application/json", bytes.NewBuffer(jsonPayload))

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error al conectar con la IA"})
			return
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		c.JSON(http.StatusOK, result)
	})

	log.Println("✅ Servidor corriendo de forma exitosa en https://ecoradar-api.onrender.com")

	// Si Render.com pasa un puerto (ej. process.env.PORT), es recomendable leerlo.
	// Por ahora mantengo el 8080 que tenías originalmente.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)

}
