package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time" // Importamos time para configurar la duración de las conexiones

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
				   IFNULL(r.estado, 'Pendiente'), IFNULL(r.fotografia, ''), IFNULL(cat.nombre_categorias, 'General')
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
			if err := rows.Scan(&rep.ID, &rep.Titulo, &rep.Descripcion, &rep.Latitud, &rep.Longitud, &rep.Estado, &rep.Fotografia, &rep.Categoria); err != nil {
				log.Println("Error leyendo fila:", err)
				continue
			}
			reportes = append(reportes, rep)
		}
		c.JSON(http.StatusOK, reportes)
	})

	// RUTA POST: Crear un nuevo reporte desde la App del Ciudadano
	r.POST("/api/reportes", func(c *gin.Context) {
		titulo := c.PostForm("titulo")
		descripcion := c.PostForm("descripcion")
		idCategorias := c.PostForm("id_categorias")
		latitud := c.PostForm("latitud")
		longitud := c.PostForm("longitud")
		idUsuario := c.PostForm("id_usuario")

		// --- MAGIA: GUARDAR LA FOTOGRAFÍA ---
		foto, errFoto := c.FormFile("fotografia")
		nombreArchivo := ""
		if errFoto == nil {
			nombreArchivo = foto.Filename
			// Guardamos el archivo físicamente en la carpeta "uploads" de tu PC / Servidor
			c.SaveUploadedFile(foto, "uploads/"+nombreArchivo)
		}

		// Insertar en MySQL
		query := `
			INSERT INTO tbreportes (titulo, descripcion, latitud, longitud, id_categorias, id_usuario, estado, fotografia)
			VALUES (?, ?, ?, ?, ?, ?, 'Pendiente', ?)
		`

		_, err := db.Exec(query, titulo, descripcion, latitud, longitud, idCategorias, idUsuario, nombreArchivo)
		if err != nil {
			log.Println("❌ Error al guardar en MySQL:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo guardar en la base de datos"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"mensaje": "Reporte creado con éxito"})
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
			"mensaje": "Login exitoso",
			"token":   "eco_token_valido_xyz123",
			"usuario": nombre,
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

	log.Println("✅ Servidor corriendo de forma exitosa en https://ecoradar-api.onrender.com")

	// Si Render.com pasa un puerto (ej. process.env.PORT), es recomendable leerlo.
	// Por ahora mantengo el 8080 que tenías originalmente.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)
}
