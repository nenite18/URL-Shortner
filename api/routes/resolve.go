package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/nenite18/URL-Shortner/api/database"
)

func ResolveURL(c *fiber.Ctx) error {
	url := c.Params("url")

	r := database.CreateClient(0)
	defer r.Close()

	value, err := r.Get(database.Ctx, url).Result()

	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": " err in connecting to Database",
		})
	}

	if value == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "short url not found in database",
		})
	}

	//newVal , err := r.Incr(database.Ctx, "counter").Result()
	//
	//rInr := database.CreateClient(1)
	//defer rInr.Close()
	//
	//_ = rInr.Incr(database.Ctx, "counter")

	return c.Redirect(value, 301)
}
